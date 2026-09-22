package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/craigh33/adk-go-bedrock/bedrock/converse"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

type fakeRuntime struct {
	request *bedrockruntime.ConverseInput
	calls   int
}

func (f *fakeRuntime) Converse(
	_ context.Context,
	request *bedrockruntime.ConverseInput,
	_ ...func(*bedrockruntime.Options),
) (*bedrockruntime.ConverseOutput, error) {
	f.request = request
	f.calls++
	return &bedrockruntime.ConverseOutput{
		Output: &bedrocktypes.ConverseOutputMemberMessage{
			Value: bedrocktypes.Message{
				Role: bedrocktypes.ConversationRoleAssistant,
				Content: []bedrocktypes.ContentBlock{
					&bedrocktypes.ContentBlockMemberText{Value: "Let us check the payment."},
				},
			},
		},
		StopReason: bedrocktypes.StopReasonEndTurn,
	}, nil
}

func (*fakeRuntime) ConverseStream(
	context.Context,
	*bedrockruntime.ConverseStreamInput,
	...func(*bedrockruntime.Options),
) (converse.StreamReader, error) {
	return nil, errors.New("unexpected streaming call")
}

type fakeEvaluator struct{}

func (fakeEvaluator) Evaluate(context.Context, *typesafe.Request) (*typesafe.Response, error) {
	return &typesafe.Response{
		Model: "jev-test",
		Answers: map[string]typesafe.Answer{
			"department": typesafe.ChoiceAnswer{Type: "choice", Choice: billing, Confidence: 0.9},
		},
	}, nil
}

func TestBedrockRouting(t *testing.T) {
	t.Parallel()
	api := &fakeRuntime{}
	llm, err := converse.NewWithAPI("test-bedrock-model", api)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newRouter(fakeEvaluator{}, llm)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(
		runner.Config{AppName: "test", Agent: a, SessionService: session.InMemoryService(), AutoCreateSession: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	var authors []string
	for event, err := range r.Run(t.Context(), "user", "session", genai.NewContentFromText("Charged twice", genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			t.Fatal(err)
		}
		authors = append(authors, event.Author)
	}
	if strings.Join(authors, ",") != "triage,billing" || api.calls != 1 {
		t.Fatalf("authors=%v calls=%d", authors, api.calls)
	}
	if api.request.ModelId == nil || *api.request.ModelId != "test-bedrock-model" || len(api.request.Messages) == 0 {
		t.Fatalf("Bedrock request=%#v", api.request)
	}
	if api.request.ToolConfig != nil {
		t.Fatal("unexpected transfer tools on routed child")
	}
}
