package contextfilter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

type evaluatorFunc func(context.Context, *typesafe.Request) (*typesafe.Response, error)

func (f evaluatorFunc) Evaluate(ctx context.Context, request *typesafe.Request) (*typesafe.Response, error) {
	return f(ctx, request)
}

type callbackContext struct {
	agent.Context

	parent context.Context
	input  *genai.Content
	sess   session.Session
}

func (c callbackContext) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c callbackContext) Done() <-chan struct{}       { return c.parent.Done() }
func (c callbackContext) Err() error                  { return c.parent.Err() }
func (c callbackContext) Value(key any) any           { return c.parent.Value(key) }
func (c callbackContext) UserContent() *genai.Content { return c.input }
func (c callbackContext) Session() session.Session    { return c.sess }

func user(text string) *genai.Content  { return genai.NewContentFromText(text, genai.RoleUser) }
func reply(text string) *genai.Content { return genai.NewContentFromText(text, genai.RoleModel) }

func scores(request *typesafe.Request) *typesafe.Response {
	answers := make(map[string]typesafe.Answer, len(request.Questions))
	for id := range request.Questions {
		answers[id] = typesafe.NoulAnswer{Type: "noul", Noul: 0}
	}
	return &typesafe.Response{Model: "jev-test", Answers: answers, Usage: typesafe.Usage{InputTokens: 10}}
}

func testConfig(api typesafe.Evaluator) Config {
	return Config{API: api, KeepRecentTurns: 1, MinBytes: 1}
}

func apply(t *testing.T, cfg Config, ctx callbackContext, request *model.LLMRequest) (Report, error) {
	t.Helper()
	var report Report
	cfg.OnReport = func(_ agent.Context, result Report) { report = result }
	filter, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	response, err := filter.BeforeModelCallback()(ctx, request)
	if response != nil {
		t.Fatal("filter must not substitute a model response")
	}
	return report, err
}

//nolint:gocognit // Each mode checks selection, request metadata and non-mutation together.
func TestWholeTurnSelection(t *testing.T) {
	t.Parallel()
	for _, observe := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "observe"}[observe], func(t *testing.T) {
			t.Parallel()
			current := user("Change the title")
			contents := []*genai.Content{
				user("Old debugging"), call("a"), result("a"), reply("Finished"),
				user("Use Go"), reply("Noted"), current, call("b"), result("b"),
				reply("Working"), user("Synthetic continuation"),
			}
			before, _ := json.Marshal(contents)
			api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
				if req.Model != "jev-pinned" || len(req.Questions) != 2 {
					t.Fatalf("unexpected request: %+v", req)
				}
				state := req.State.(reviewState)
				if !strings.Contains(state.LatestRequest, "Change the title") ||
					!strings.Contains(state.Instructions, "Be precise") {
					t.Fatalf("missing request or instructions: %+v", state)
				}
				if !strings.Contains(state.Groups[0].Text, `"result":"lookup"`) {
					t.Fatal("tool result absent from review")
				}
				response := scores(req)
				response.Answers["4"] = typesafe.NoulAnswer{Noul: 0.1}
				return response, nil
			})
			cfg := testConfig(api)
			cfg.Observe, cfg.Model = observe, "jev-pinned"
			request := &model.LLMRequest{
				Contents: contents,
				Config:   &genai.GenerateContentConfig{SystemInstruction: user("Be precise")},
			}
			report, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request)
			if err != nil || report.Err != nil || len(report.Decisions) != 2 || !report.Decisions[0].Remove ||
				report.Decisions[1].Remove {
				t.Fatalf("report=%+v err=%v", report, err)
			}
			want, removed := contents[4:], 1
			if observe {
				want, removed = contents, 0
			}
			if !reflect.DeepEqual(request.Contents, want) || report.RemovedTurns != removed ||
				report.Usage.InputTokens != 10 {
				t.Fatalf("wrong selection: %+v", report)
			}
			after, _ := json.Marshal(contents)
			if string(before) != string(after) {
				t.Fatal("original messages were mutated")
			}
			for i := range want {
				if request.Contents[i] != want[i] {
					t.Fatal("retained message was reconstructed")
				}
			}
		})
	}
}

func TestInvalidAnswersRetainAffectedTurns(t *testing.T) {
	t.Parallel()

	current := user("Current")
	contents := []*genai.Content{user("First"), reply("One"), user("Second"), reply("Two"), current}
	api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		response := scores(req)
		delete(response.Answers, "0")
		return response, nil
	})
	request := &model.LLMRequest{Contents: contents}
	report, err := apply(t, testConfig(api), callbackContext{parent: t.Context(), input: current}, request)
	want := []*genai.Content{contents[0], contents[1], current}
	if err != nil || report.Err == nil || !reflect.DeepEqual(request.Contents, want) {
		t.Fatalf("missing answer: report=%+v err=%v", report, err)
	}
}

//nolint:gocognit // Exercise distinct evaluator failures through the plugin callback.
func TestReviewFailuresAndCancellation(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"error", "nil", "timeout", "cancel", "already-canceled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			current := user("Current")
			contents := []*genai.Content{user("Old"), reply("Done"), current}
			calls := 0
			api := evaluatorFunc(func(ctx context.Context, req *typesafe.Request) (*typesafe.Response, error) {
				calls++
				switch mode {
				case "cancel":
					cancel()
					return scores(req), nil
				case "timeout":
					<-ctx.Done()
					return scores(req), nil // Even a late successful result must be ignored.
				case "nil":
					return nil, nil //nolint:nilnil // Deliberately invalid evaluator response.
				default:
					return nil, errors.New("unavailable")
				}
			})
			cfg := testConfig(api)
			cfg.Timeout = time.Millisecond
			if mode == "already-canceled" {
				cancel()
			}
			request := &model.LLMRequest{Contents: contents}
			report, err := apply(t, cfg, callbackContext{parent: parent, input: current}, request)
			if !reflect.DeepEqual(request.Contents, contents) {
				t.Fatal("failure discarded context")
			}
			if mode == "cancel" || mode == "already-canceled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation swallowed: %v", err)
				}
			} else if err != nil || report.Err == nil {
				t.Fatalf("review failure not reported: report=%+v err=%v", report, err)
			}
			if mode == "already-canceled" && calls != 0 {
				t.Fatal("called evaluator after cancellation")
			}
		})
	}
}

func TestSkipsShortOrUnknownContext(t *testing.T) {
	t.Parallel()
	current := user("Current")
	api := evaluatorFunc(func(context.Context, *typesafe.Request) (*typesafe.Response, error) {
		t.Fatal("unexpected API call")
		return nil, errors.New("unexpected call")
	})
	for _, contents := range [][]*genai.Content{
		{}, {current}, {user("Old"), reply("Done"), current},
	} {
		request := &model.LLMRequest{Contents: contents}
		report, err := apply(t, Config{API: api}, callbackContext{parent: t.Context(), input: current}, request)
		if err != nil || report.Err != nil || !reflect.DeepEqual(request.Contents, contents) {
			t.Fatal("short context should be unchanged")
		}
	}
	request := &model.LLMRequest{Contents: []*genai.Content{user("Old"), user("Delegated input")}}
	if _, err := apply(t, testConfig(api), callbackContext{parent: t.Context(), input: current}, request); err != nil {
		t.Fatal(err)
	}
	current = &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{InlineData: &genai.Blob{MIMEType: "image/png"}}},
	}
	request.Contents = []*genai.Content{user("Old"), reply("Done"), current}
	if _, err := apply(t, testConfig(api), callbackContext{parent: t.Context(), input: current}, request); err != nil {
		t.Fatal(err)
	}
	current = user("Only current turn")
	request.Contents = []*genai.Content{current}
	if _, err := apply(t, testConfig(api), callbackContext{parent: t.Context(), input: current}, request); err != nil {
		t.Fatal(err)
	}
}

func TestSingleEvaluationAndRequestLimit(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{1, defaultMaxRequestBytes} {
		calls := 0
		api := evaluatorFunc(func(ctx context.Context, req *typesafe.Request) (*typesafe.Response, error) {
			calls++
			data, err := json.Marshal(req)
			if err != nil || len(data) > limit || len(req.Questions) != 2 {
				t.Fatalf("unexpected evaluation: bytes=%d limit=%d questions=%d", len(data), limit, len(req.Questions))
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("evaluation has no deadline")
			}
			return scores(req), nil
		})
		current := user("Current")
		contents := []*genai.Content{user("First"), reply("One"), user("Second"), reply("Two"), current}
		request := &model.LLMRequest{Contents: contents}
		cfg := testConfig(api)
		cfg.MaxRequestBytes = limit
		report, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request)
		if err != nil {
			t.Fatal(err)
		}
		if limit == 1 {
			if calls != 0 || report.Err == nil || !reflect.DeepEqual(request.Contents, contents) {
				t.Fatal("oversized request must retain history without calling Jev")
			}
		} else if calls != 1 || report.Err != nil || report.RemovedTurns != 2 || len(request.Contents) != 1 {
			t.Fatalf("expected one evaluation for both turns: calls=%d report=%+v", calls, report)
		}
	}
}
