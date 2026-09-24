package contextfilter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestBatchBudgetAndPartialFailure(t *testing.T) {
	t.Parallel()
	current := user("Current")
	contents := []*genai.Content{
		user("First"),
		reply("One"),
		user("Second"),
		reply("Two"),
		user("Third"),
		reply("Three"),
		current,
	}
	ctx := callbackContext{parent: t.Context(), input: current}
	calls, limit := 0, 0
	var deadline time.Time
	api := evaluatorFunc(func(ctx context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		calls++
		currentDeadline, ok := ctx.Deadline()
		if !ok || (!deadline.IsZero() && !deadline.Equal(currentDeadline)) {
			t.Fatal("batches must share a deadline")
		}
		deadline = currentDeadline
		data, err := json.Marshal(req)
		if err != nil || len(data) > limit || len(req.Questions) != 1 {
			t.Fatalf("unbounded batch: bytes=%d limit=%d questions=%d", len(data), limit, len(req.Questions))
		}
		if calls == 2 {
			return nil, errors.New("failed second batch")
		}
		return scores(req, 0), nil
	})
	cfg, _ := configure(testConfig(api))
	groups := groupTurns(ctx, contents, cfg)
	state := mappers.ContextReviewState{LatestRequest: mappers.ContextContent(current).Text}
	for i := range 3 {
		data, _ := json.Marshal((&contextFilter{cfg: cfg}).batchRequest(groups, []int{i}, state))
		limit = max(limit, len(data))
	}
	cfg.MaxBatchBytes = limit
	request := &model.LLMRequest{Contents: contents}
	report, err := apply(t, cfg, ctx, request)
	want := []*genai.Content{contents[2], contents[3], current}
	if err != nil || report.Err == nil || calls != 3 || !reflect.DeepEqual(request.Contents, want) ||
		report.Usage.InputTokens != 20 {
		t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
	}
	cfg.MaxBatchBytes = 1
	request.Contents = contents
	report, err = apply(t, cfg, ctx, request)
	if err != nil || report.Err == nil || calls != 3 || !reflect.DeepEqual(request.Contents, contents) {
		t.Fatal("oversized context should be retained without an API call")
	}
}
