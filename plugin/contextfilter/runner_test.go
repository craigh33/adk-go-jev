package contextfilter

import (
	"context"
	"iter"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

type recordingModel struct {
	requests [][]*genai.Content
}

func (m *recordingModel) Name() string { return "test" }

func (m *recordingModel) GenerateContent(
	_ context.Context,
	req *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.requests = append(m.requests, req.Contents)
		yield(&model.LLMResponse{Content: reply("Understood"), TurnComplete: true}, nil)
	}
}

func TestRunnerRestoresHistoryOnLaterTurn(t *testing.T) {
	t.Parallel()
	api := evaluatorFunc(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		probability := 0.0
		if strings.Contains(req.State.(mappers.ContextReviewState).LatestRequest, "Return") {
			probability = 1
		}
		return scores(req, probability), nil
	})
	filter, err := New(testConfig(api))
	if err != nil {
		t.Fatal(err)
	}
	var beforeFilter, afterFilter []int
	assessment, err := plugin.New(plugin.Config{
		Name: "assessment",
		BeforeModelCallback: func(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
			beforeFilter = append(beforeFilter, len(req.Contents))
			return nil, ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	llm := &recordingModel{}
	a, err := llmagent.New(
		llmagent.Config{Name: "assistant", Model: llm, BeforeModelCallbacks: []llmagent.BeforeModelCallback{
			func(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
				afterFilter = append(afterFilter, len(req.Contents))
				return nil, ctx.Err()
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName: "filter-test", Agent: a, SessionService: svc, AutoCreateSession: true,
		PluginConfig: runner.PluginConfig{Plugins: []*plugin.Plugin{assessment, filter}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"Remember the task tracker uses Go", "Unrelated arithmetic", "Return to the task tracker"} {
		for _, err := range r.Run(t.Context(), "user", "session", user(prompt), agent.RunConfig{}) {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(llm.requests) != 3 || len(llm.requests[1]) != 1 || len(llm.requests[2]) != 5 {
		t.Fatalf("unexpected model requests: %+v", llm.requests)
	}
	if !reflect.DeepEqual(beforeFilter, []int{1, 3, 5}) || !reflect.DeepEqual(afterFilter, []int{1, 1, 5}) {
		t.Fatalf("unexpected callback ordering: before=%v after=%v", beforeFilter, afterFilter)
	}
	if llm.requests[2][0].Parts[0].Text != "Remember the task tracker uses Go" {
		t.Fatal("old turn did not return")
	}
	saved, err := svc.Get(
		t.Context(),
		&session.GetRequest{AppName: "filter-test", UserID: "user", SessionID: "session"},
	)
	if err != nil || saved.Session.Events().Len() != 6 {
		t.Fatalf("saved history lost events: %v", err)
	}
}
