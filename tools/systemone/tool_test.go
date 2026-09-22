package systemone

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool/toolconfirmation"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

type fakeAPI struct {
	request  *typesafe.Request
	ctx      context.Context
	response *typesafe.Response
	err      error
}

func (f *fakeAPI) Evaluate(ctx context.Context, request *typesafe.Request) (*typesafe.Response, error) {
	f.request, f.ctx = request, ctx
	return f.response, f.err
}

type testContext struct{ agent.StrictContextMock }

func (c *testContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }

type runnableTool interface {
	Run(agent.Context, any) (map[string]any, error)
	ProcessRequest(agent.Context, *model.LLMRequest) error
}

func TestToolRoundTrip(t *testing.T) {
	t.Parallel()
	var response typesafe.Response
	if err := json.Unmarshal(
		[]byte(
			`{"model":"jev-test","answers":{"urgent":{"type":"noul","noul":0}},"usage":{"input_tokens":1,"output_tokens":0}}`,
		),
		&response,
	); err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{response: &response}
	value, err := New(
		Config{
			API:         api,
			Name:        "assess_urgency",
			Description: "Check urgency",
			Model:       "jev-pinned",
			Questions:   map[string]typesafe.Question{"urgent": typesafe.Noul{Instructions: "Urgent?"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := value.(runnableTool)
	if !ok {
		t.Fatal("tool is not executable by ADK")
	}
	ctx := &testContext{StrictContextMock: agent.StrictContextMock{Ctx: t.Context()}}
	req := &model.LLMRequest{}
	if err := tool.ProcessRequest(ctx, req); err != nil {
		t.Fatal(err)
	}
	if req.Tools["assess_urgency"] != value || req.Config == nil || len(req.Config.Tools) != 1 {
		t.Fatal("tool not registered in the LLM request")
	}
	decl := req.Config.Tools[0].FunctionDeclarations[0]
	data, err := json.Marshal(decl.ParametersJsonSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	json.Unmarshal(data, &schema)
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) != 1 || props["state"] == nil {
		t.Fatalf("unexpected generated input schema: %s", data)
	}
	result, err := tool.Run(ctx, map[string]any{"state": "No rush"})
	if err != nil {
		t.Fatal(err)
	}
	if api.ctx != ctx || api.request.State != "No rush" || api.request.Model != "jev-pinned" ||
		len(api.request.Questions) != 1 {
		t.Fatalf("wrong evaluation request: %#v", api.request)
	}
	data, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got typesafe.Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	answer, ok := got.Answers["urgent"].(typesafe.NoulAnswer)
	if !ok || answer.Noul != 0 || got.Model != "jev-test" || got.Usage.OutputTokens != 0 {
		t.Fatalf("lost response fields: %s", data)
	}
	for _, args := range []map[string]any{{}, {"state": 3}, {"state": "text", "questions": map[string]any{}}} {
		if _, err := tool.Run(ctx, args); err == nil {
			t.Errorf("accepted invalid arguments: %v", args)
		}
	}
}

func TestToolErrors(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("nil API accepted")
	}
	if _, err := New(Config{API: &fakeAPI{}}); err == nil {
		t.Fatal("empty questions accepted")
	}
	failure := errors.New("evaluation failed")
	for _, upstreamError := range []error{failure, nil} {
		api := &fakeAPI{err: upstreamError}
		value, err := New(
			Config{API: api, Questions: map[string]typesafe.Question{"a": typesafe.Noul{Instructions: "Test?"}}},
		)
		if err != nil {
			t.Fatal(err)
		}
		tool, ok := value.(runnableTool)
		if !ok {
			t.Fatal("tool cannot run")
		}
		ctx := &testContext{StrictContextMock: agent.StrictContextMock{Ctx: t.Context()}}
		_, err = tool.Run(ctx, map[string]any{"state": "text"})
		if err == nil || (upstreamError != nil && !errors.Is(err, failure)) {
			t.Fatalf("upstream error/nil response lost: %v", err)
		}
	}
}
