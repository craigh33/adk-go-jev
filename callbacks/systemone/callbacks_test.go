package systemone

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"math"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

type fakeAPI struct {
	request  *typesafe.Request
	calls    int
	response *typesafe.Response
	err      error
}

func (f *fakeAPI) Evaluate(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
	f.request = req
	f.calls++
	if f.response != nil {
		return f.response, f.err
	}
	return &typesafe.Response{
		Model:   "jev-test",
		Answers: map[string]typesafe.Answer{"allowed": typesafe.NoulAnswer{Type: "noul", Noul: 0}},
	}, f.err
}

type fakeModel struct {
	calls    int
	toolCall bool
	partial  bool
}

func (f *fakeModel) Name() string { return "test-model" }
func (f *fakeModel) GenerateContent(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		f.calls++
		content := genai.NewContentFromText("original output", genai.RoleModel)
		if f.toolCall && f.calls == 1 {
			content = &genai.Content{
				Role: genai.RoleModel,
				Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "draft", Args: map[string]any{"text": "draft text"}}},
				},
			}
		}
		yield(&model.LLMResponse{Content: content, Partial: f.partial}, nil)
	}
}

func config(api typesafe.Evaluator, block bool) Config {
	return Config{
		API:       api,
		Model:     "jev-pinned",
		Questions: map[string]typesafe.Question{"allowed": typesafe.Noul{Instructions: "Allowed?"}},
		OutputKey: "assessment",
		Policy: func(agent.Context, *typesafe.Response) (Decision, error) {
			return Decision{Block: block, Reason: "Needs review"}, nil
		},
	}
}

func runModel(t *testing.T, cfg llmagent.Config) ([]*session.Event, session.State, error) {
	t.Helper()
	a, err := llmagent.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc := session.InMemoryService()
	r, err := runner.New(runner.Config{AppName: "test", Agent: a, SessionService: svc, AutoCreateSession: true})
	if err != nil {
		t.Fatal(err)
	}
	var events []*session.Event
	var runErr error
	for event, err := range r.Run(t.Context(), "user", "session", genai.NewContentFromText("Please help", genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			runErr = err
			break
		}
		events = append(events, event)
	}
	saved, err := svc.Get(t.Context(), &session.GetRequest{AppName: "test", UserID: "user", SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	return events, saved.Session.State(), runErr
}

func eventTexts(events []*session.Event) string {
	var text strings.Builder
	for _, event := range events {
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String()
}

//nolint:gocognit // Each table case verifies the complete observable result.
func TestModelCallbacks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		after, block bool
		wantCalls    int
		wantText     string
	}{
		{"before block", false, true, 0, "Needs review"},
		{"before allow", false, false, 1, "original output"},
		{"after block", true, true, 1, "Needs review"},
		{"after allow", true, false, 1, "original output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api, llm := &fakeAPI{}, &fakeModel{}
			cfg := llmagent.Config{Name: "assistant", Model: llm}
			if tc.after {
				cb, err := AfterModel(config(api, tc.block))
				if err != nil {
					t.Fatal(err)
				}
				cfg.AfterModelCallbacks = []llmagent.AfterModelCallback{cb}
			} else {
				cb, err := BeforeModel(config(api, tc.block))
				if err != nil {
					t.Fatal(err)
				}
				cfg.BeforeModelCallbacks = []llmagent.BeforeModelCallback{cb}
			}
			events, state, err := runModel(t, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if llm.calls != tc.wantCalls || eventTexts(events) != tc.wantText {
				t.Fatalf("calls=%d text=%q", llm.calls, eventTexts(events))
			}
			if api.calls != 1 || api.request.Model != "jev-pinned" {
				t.Fatalf("API calls=%d request=%#v", api.calls, api.request)
			}
			record, err := state.Get("assessment")
			if err != nil {
				t.Fatal(err)
			}
			m, ok := record.(map[string]any)
			if !ok || !reflect.DeepEqual(m["decision"], map[string]any{"block": tc.block, "reason": "Needs review"}) {
				t.Fatalf("record=%#v", record)
			}
		})
	}
}

func TestToolCallbackPreventsExecution(t *testing.T) {
	t.Parallel()
	for _, block := range []bool{false, true} {
		t.Run(map[bool]string{false: "allow", true: "block"}[block], func(t *testing.T) {
			t.Parallel()
			api := &fakeAPI{}
			cb, err := BeforeTool(config(api, block))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			target, err := functiontool.New(
				functiontool.Config{Name: "draft", Description: "Draft text"},
				func(_ agent.Context, args struct {
					Text string `json:"text"`
				}) (map[string]any, error) {
					calls++
					return map[string]any{"text": args.Text}, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			events, _, err := runModel(
				t,
				llmagent.Config{
					Name:                "assistant",
					Model:               &fakeModel{toolCall: true},
					Tools:               []tool.Tool{target},
					BeforeToolCallbacks: []llmagent.BeforeToolCallback{cb},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 1
			if block {
				wantCalls = 0
			}
			if calls != wantCalls || api.calls != 1 {
				t.Fatalf("tool calls=%d API calls=%d", calls, api.calls)
			}
			if !reflect.DeepEqual(
				api.request.State,
				map[string]any{"tool": "draft", "arguments": map[string]any{"text": "draft text"}},
			) {
				t.Fatalf("state=%#v", api.request.State)
			}
			assertToolResult(t, events, block)
		})
	}
}

func assertToolResult(t *testing.T, events []*session.Event, block bool) {
	t.Helper()
	for _, event := range events {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if result := part.FunctionResponse; result != nil && result.Name == "draft" {
				if block && result.Response["blocked"] != true {
					t.Fatalf("result=%v", result.Response)
				}
				if !block && result.Response["text"] != "draft text" {
					t.Fatalf("result=%v", result.Response)
				}
				return
			}
		}
	}
	t.Fatal("missing tool result")
}

func TestAssessmentErrorsStopModel(t *testing.T) {
	t.Parallel()
	failure := errors.New("failed")
	for _, policyError := range []bool{false, true} {
		api, llm := &fakeAPI{}, &fakeModel{}
		cfg := config(api, false)
		if policyError {
			cfg.Policy = func(agent.Context, *typesafe.Response) (Decision, error) { return Decision{}, failure }
		} else {
			api.err = failure
		}
		cb, err := BeforeModel(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = runModel(
			t,
			llmagent.Config{Name: "assistant", Model: llm, BeforeModelCallbacks: []llmagent.BeforeModelCallback{cb}},
		)
		if !errors.Is(err, failure) || llm.calls != 0 {
			t.Fatalf("calls=%d err=%v", llm.calls, err)
		}
	}
}

func TestOutputRejectsPartialResponses(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	cb, err := AfterModel(config(api, false))
	if err != nil {
		t.Fatal(err)
	}
	events, _, err := runModel(
		t,
		llmagent.Config{
			Name:                "assistant",
			Model:               &fakeModel{partial: true},
			AfterModelCallbacks: []llmagent.AfterModelCallback{cb},
		},
	)
	if err == nil || len(events) != 0 || api.calls != 0 {
		t.Fatalf("events=%v calls=%d err=%v", events, api.calls, err)
	}
	failure := errors.New("model failed")
	if _, err := cb(nil, nil, failure); !errors.Is(err, failure) {
		t.Fatalf("lost original error: %v", err)
	}
}

func TestCallbackConfiguration(t *testing.T) {
	t.Parallel()
	for _, cfg := range []Config{{}, {API: &fakeAPI{}}, {Policy: func(agent.Context, *typesafe.Response) (Decision, error) { return Decision{}, nil }}} {
		if _, err := BeforeModel(cfg); err == nil {
			t.Fatal("accepted invalid before-model config")
		}
		if _, err := AfterModel(cfg); err == nil {
			t.Fatal("accepted invalid after-model config")
		}
		if _, err := BeforeTool(cfg); err == nil {
			t.Fatal("accepted invalid before-tool config")
		}
	}
}

func TestMappingErrorsIdentifyCallback(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{response: &typesafe.Response{
		Answers: map[string]typesafe.Answer{"allowed": typesafe.NoulAnswer{Noul: math.NaN()}},
	}}
	cfg := config(api, false)
	_, err := cfg.assess(&agent.StrictContextMock{Ctx: t.Context()}, "Input")
	var cause *json.UnsupportedValueError
	if err == nil || !strings.Contains(err.Error(), "systemone callback: map assessment:") || !errors.As(err, &cause) {
		t.Fatalf("lost caller context or encoding error: %v", err)
	}
}
