package contextfilter

import (
	"strings"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestRequestIncludesPinnedEvidence(t *testing.T) {
	t.Parallel()
	p := &contextFilter{cfg: Config{Model: "jev-version"}}
	request := p.newRequest(
		&model.LLMRequest{Config: &genai.GenerateContentConfig{SystemInstruction: user("Answer briefly")}},
		[]turn{{start: 0, text: "Use SQLite"}, {start: 2, pinned: true, text: "Current task"}},
		"Which database did we choose?",
	)
	state := request.State.(reviewState)
	if request.Model != "jev-version" || state.LatestRequest != "Which database did we choose?" ||
		!strings.Contains(state.Instructions, "Answer briefly") || len(state.Groups) != 2 || !state.Groups[1].Pinned {
		t.Fatalf("lost review evidence: %+v", request)
	}
	if len(request.Questions) != 1 {
		t.Fatalf("pinned turn offered for removal: %+v", request.Questions)
	}
	question, ok := request.Questions["0"].(typesafe.Noul)
	instructions, _ := question.Instructions.(string)
	if !ok || !strings.Contains(instructions, "group 0") || question.Criteria == nil ||
		question.Criteria.True == "" || question.Criteria.False == "" {
		t.Fatalf("missing relevance criteria: %+v", request.Questions)
	}
}
