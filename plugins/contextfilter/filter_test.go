package contextfilter

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func batchContents() []*genai.Content {
	return []*genai.Content{
		user("First " + strings.Repeat(`"λ"`, 256)), reply("One"),
		user("Second " + strings.Repeat(`"λ"`, 256)), reply("Two"),
		user("Third " + strings.Repeat(`"λ"`, 256)), reply("Three"),
		user("Current request"),
	}
}

func TestBatchesPreserveEvidenceAndOrder(t *testing.T) {
	t.Parallel()
	contents := append([]*genai.Content{
		{Role: "system", Parts: []*genai.Part{{Text: "Standing constraint"}}},
	}, batchContents()...)
	current := contents[len(contents)-1]
	before, _ := json.Marshal(contents)
	calls := 0
	api := evaluatorFunc(func(ctx context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		id := strconv.Itoa(1 + 2*calls)
		calls++
		data, err := json.Marshal(req)
		if err != nil || len(data) > 4096 || len(req.Questions) != 1 || req.Questions[id] == nil {
			t.Fatalf("batch %d: bytes=%d questions=%v err=%v", calls, len(data), req.Questions, err)
		}
		state := req.State.(reviewState)
		if req.Model != "jev-version" || !strings.Contains(state.LatestRequest, "Current request") ||
			!strings.Contains(state.Instructions, "Answer briefly") || len(state.Groups) != 3 {
			t.Fatalf("lost batch evidence: %+v", state)
		}
		if !state.Groups[0].Pinned || state.Groups[0].ID != "0" ||
			state.Groups[1].ID != id || state.Groups[1].Pinned ||
			!state.Groups[2].Pinned || state.Groups[2].ID != "7" {
			t.Fatalf("incorrect batch groups: %+v", state.Groups)
		}
		question := req.Questions[id].(typesafe.Noul)
		if question.Criteria == nil || question.Criteria.True == "" || question.Criteria.False == "" {
			t.Fatal("missing relevance criteria")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing review deadline")
		}
		response := scores(req)
		response.Usage.OutputTokens = 1
		return response, nil
	})
	cfg := testConfig(api)
	cfg.Model, cfg.MaxRequestBytes = "jev-version", 4096
	request := &model.LLMRequest{
		Contents: contents,
		Config:   &genai.GenerateContentConfig{SystemInstruction: user("Answer briefly")},
	}
	report, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request)
	if err != nil || report.Err != nil || calls != 3 || report.RemovedTurns != 3 ||
		report.Usage != (typesafe.Usage{InputTokens: 30, OutputTokens: 3}) {
		t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
	}
	wantDecisions := []Decision{
		{Start: 1, End: 3, Remove: true},
		{Start: 3, End: 5, Remove: true},
		{Start: 5, End: 7, Remove: true},
	}
	if !slices.Equal(report.Decisions, wantDecisions) {
		t.Fatalf("decisions out of original order: %+v", report.Decisions)
	}
	if !slices.Equal(request.Contents, []*genai.Content{contents[0], current}) {
		t.Fatal("retained messages were changed")
	}
	after, _ := json.Marshal(contents)
	if string(before) != string(after) {
		t.Fatal("original history was changed")
	}
}

func TestExactRequestByteLimit(t *testing.T) {
	t.Parallel()
	contents := []*genai.Content{user(`A "quoted" λ request`), reply("Done"), user("Current")}
	current := contents[2]
	size, calls := 0, 0
	api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		size = len(data)
		calls++
		return scores(req), nil
	})
	cfg := testConfig(api)
	request := &model.LLMRequest{Contents: contents}
	if _, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{size, size - 1} {
		cfg.MaxRequestBytes, calls = limit, 0
		request.Contents = contents
		report, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request)
		if err != nil {
			t.Fatal(err)
		}
		if limit == size {
			if calls != 1 || report.Err != nil || report.RemovedTurns != 1 {
				t.Fatalf("exact limit rejected: calls=%d report=%+v", calls, report)
			}
		} else if calls != 0 || report.Err == nil || !slices.Equal(request.Contents, contents) {
			t.Fatalf("over-limit turn was evaluated or removed: calls=%d report=%+v", calls, report)
		}
	}
}

func TestOversizedTurnRetainedBetweenBatches(t *testing.T) {
	t.Parallel()
	for _, position := range []int{0, 2} {
		contents := []*genai.Content{user("First"), reply("One"), user("Second"), reply("Two"), user("Current")}
		oversized := []*genai.Content{user(strings.Repeat("Oversized", 1024)), reply("Keep this reply too")}
		contents = slices.Insert(contents, position, oversized...)
		current := contents[len(contents)-1]
		api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
			data, err := json.Marshal(req)
			if err != nil || len(data) > 4096 || req.Questions[strconv.Itoa(position)] != nil ||
				strings.Contains(string(data), "Oversized") {
				t.Fatal("oversized turn was truncated or submitted")
			}
			return scores(req), nil
		})
		cfg := testConfig(api)
		cfg.MaxRequestBytes = 4096
		request := &model.LLMRequest{Contents: contents}
		report, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request)
		if err != nil || report.Err == nil || report.RemovedTurns != 2 ||
			!slices.Equal(request.Contents, []*genai.Content{oversized[0], oversized[1], current}) {
			t.Fatalf("position=%d report=%+v err=%v", position, report, err)
		}
	}
}

//nolint:gocognit // Compare partial success and cancellation across the same three batches.
func TestBatchFailureStopsReview(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"error", "nil", "timeout", "cancel", "observe"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			contents := batchContents()
			current := contents[len(contents)-1]
			var deadline time.Time
			calls := 0
			api := evaluatorFunc(func(ctx context.Context, req *typesafe.Request) (*typesafe.Response, error) {
				calls++
				if len(req.Questions) != 1 {
					t.Fatal("expected one turn per batch")
				}
				if calls == 1 {
					deadline, _ = ctx.Deadline()
					return scores(req), nil
				}
				if calls > 2 {
					t.Fatal("continued after a failed batch")
				}
				if got, _ := ctx.Deadline(); got.IsZero() || !got.Equal(deadline) {
					t.Fatalf("deadline reset between batches: %v != %v", got, deadline)
				}
				switch mode {
				case "cancel":
					cancel()
					return scores(req), nil
				case "timeout":
					<-ctx.Done()
					return scores(req), nil
				case "nil":
					return nil, nil //nolint:nilnil // Deliberately invalid evaluator response.
				default:
					return scores(req), errors.New("unavailable")
				}
			})
			cfg := testConfig(api)
			cfg.MaxRequestBytes, cfg.Timeout, cfg.Observe = 4096, time.Second, mode == "observe"
			request := &model.LLMRequest{Contents: contents}
			report, err := apply(t, cfg, callbackContext{parent: parent, input: current}, request)
			if calls != 2 || report.Err == nil || len(report.Decisions) != 1 || !report.Decisions[0].Remove {
				t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
			}
			want := contents[2:]
			removed, tokens := 1, 20
			if mode == "nil" {
				tokens = 10
			}
			if mode == "cancel" || mode == "observe" {
				want, removed = contents, 0
			}
			if !slices.Equal(request.Contents, want) || report.RemovedTurns != removed ||
				report.Usage.InputTokens != tokens {
				t.Fatalf("incorrect partial result: %+v", report)
			}
			if mode == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("parent cancellation swallowed: %v", err)
				}
			} else if err != nil {
				t.Fatalf("review failure stopped the model call: %v", err)
			}
			if mode == "timeout" && !errors.Is(report.Err, context.DeadlineExceeded) {
				t.Fatalf("deadline failure missing: %v", report.Err)
			}
		})
	}
}

func TestInvalidBatchAnswerDoesNotStopReview(t *testing.T) {
	t.Parallel()
	contents := batchContents()
	current := contents[len(contents)-1]
	calls := 0
	api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		calls++
		response := scores(req)
		if calls == 2 {
			response.Answers["2"] = typesafe.NoulAnswer{Noul: math.NaN()}
		}
		return response, nil
	})
	cfg := testConfig(api)
	cfg.MaxRequestBytes = 4096
	request := &model.LLMRequest{Contents: contents}
	report, err := apply(t, cfg, callbackContext{parent: t.Context(), input: current}, request)
	if err != nil || report.Err == nil || calls != 3 || report.RemovedTurns != 2 ||
		!slices.Equal(request.Contents, []*genai.Content{contents[2], contents[3], current}) {
		t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
	}
}

func TestFilteredHistoryReturnsOnLaterRequest(t *testing.T) {
	t.Parallel()
	history := []*genai.Content{user("Use Go and SQLite"), reply("Agreed")}
	before, _ := json.Marshal(history)
	calls, reports := 0, 0
	api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		calls++
		state := req.State.(reviewState)
		if len(state.Groups) != 2 || !strings.Contains(state.Groups[0].Text, "Go and SQLite") {
			t.Fatal("previously filtered history was not reconsidered")
		}
		response := scores(req)
		if strings.Contains(state.LatestRequest, "task tracker") {
			response.Answers["0"] = typesafe.NoulAnswer{Noul: 1}
		}
		return response, nil
	})
	cfg := testConfig(api)
	cfg.OnReport = func(_ agent.Context, report Report) {
		reports++
		if report.Err != nil {
			t.Fatal(report.Err)
		}
	}
	filter, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, prompt := range []string{"An unrelated question", "Back to our task tracker: what did we agree?"} {
		current := user(prompt)
		contents := append(slices.Clone(history), current)
		request := &model.LLMRequest{Contents: contents}
		ctx := callbackContext{parent: t.Context(), input: current}
		if response, err := filter.BeforeModelCallback()(ctx, request); response != nil || err != nil {
			t.Fatalf("response=%v err=%v", response, err)
		}
		want := contents
		if i == 0 {
			want = contents[2:]
		}
		if !slices.Equal(request.Contents, want) {
			t.Fatalf("wrong history for request %d", i)
		}
	}
	after, _ := json.Marshal(history)
	if calls != 2 || reports != 2 || string(before) != string(after) {
		t.Fatal("filter retained invocation state or changed the original history")
	}
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

func TestOneBatchAndProtectedContextLimit(t *testing.T) {
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
