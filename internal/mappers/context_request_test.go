package mappers

import (
	"reflect"
	"testing"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestContextReviewRequest(t *testing.T) {
	t.Parallel()
	state := ContextReviewState{
		LatestRequest: "Which database did we choose?",
		Instructions:  "Answer briefly",
		Groups: []ContextReviewGroup{
			{ID: "0", Text: "Use SQLite"},
			{ID: "2", Pinned: true, Text: "Current task"},
		},
	}
	request := ContextReviewRequest("jev-version", state)
	if request.Model != "jev-version" || !reflect.DeepEqual(request.State, state) {
		t.Fatalf("lost review evidence: %+v", request)
	}
	if len(request.Questions) != 1 {
		t.Fatalf("pinned group offered for removal: %+v", request.Questions)
	}
	question, ok := request.Questions["0"].(typesafe.Noul)
	if !ok || question.Criteria == nil || question.Criteria.True == "" || question.Criteria.False == "" {
		t.Fatalf("missing relevance criteria: %+v", request.Questions)
	}
}
