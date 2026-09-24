package mappers

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestResponseMap(t *testing.T) {
	t.Parallel()
	response := &typesafe.Response{
		Model: "jev-test", Answers: map[string]typesafe.Answer{"a": typesafe.NoulAnswer{Type: "noul", Noul: 0}},
		Usage: typesafe.Usage{InputTokens: 1, OutputTokens: 0},
	}
	result, err := ResponseMap(response)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got typesafe.Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	answer, ok := got.Answers["a"].(typesafe.NoulAnswer)
	if !ok || answer.Noul != 0 || got.Model != response.Model || got.Usage != response.Usage {
		t.Fatalf("response fields lost: %+v", got)
	}
	if _, err := ResponseMap(nil); err == nil {
		t.Fatal("accepted nil response")
	}
	response.Answers["a"] = typesafe.NoulAnswer{Noul: math.NaN()}
	if _, err := ResponseMap(response); err == nil {
		t.Fatal("accepted non-JSON response")
	}
}

func TestNoulProbability(t *testing.T) {
	t.Parallel()
	for _, probability := range []float64{0, 0.1, 1} {
		got, err := NoulProbability(typesafe.NoulAnswer{Noul: probability})
		if err != nil || got != probability {
			t.Fatalf("probability=%v: got=%v err=%v", probability, got, err)
		}
	}
	for _, answer := range []typesafe.Answer{
		nil, typesafe.ChoiceAnswer{}, typesafe.NoulAnswer{Noul: math.NaN()},
		typesafe.NoulAnswer{Noul: math.Inf(1)}, typesafe.NoulAnswer{Noul: -0.1}, typesafe.NoulAnswer{Noul: 1.1},
	} {
		if _, err := NoulProbability(answer); err == nil {
			t.Fatalf("accepted invalid probability: %v", answer)
		}
	}
}

func TestTextPreservesRolesAndSource(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"", genai.RoleUser, genai.RoleModel, "system", "developer", "custom"} {
		content := &genai.Content{Role: role, Parts: []*genai.Part{
			{Text: "Find the record"},
			{FunctionCall: &genai.FunctionCall{ID: "a", Name: "lookup", Args: map[string]any{"id": 1}}},
			{
				FunctionResponse: &genai.FunctionResponse{
					ID:       "a",
					Name:     "lookup",
					Response: map[string]any{"found": true},
				},
			},
			{
				FunctionCall: &genai.FunctionCall{
					ID:   "b",
					Name: "combined",
					Args: map[string]any{"query": "constraints"},
				},
				FunctionResponse: &genai.FunctionResponse{
					ID:       "b",
					Name:     "combined",
					Response: map[string]any{"value": "Go"},
				},
			},
		}}
		before, err := json.Marshal(content)
		if err != nil {
			t.Fatal(err)
		}
		projection := ContentText(content)
		if projection.Incomplete || !strings.HasPrefix(projection.Text, role+":\nFind the record\n") ||
			!strings.Contains(projection.Text, `"arguments":{"id":1}`) ||
			!strings.Contains(projection.Text, `"response":{"found":true}`) ||
			!strings.Contains(projection.Text, `"arguments":{"query":"constraints"}`) ||
			!strings.Contains(projection.Text, `"response":{"value":"Go"}`) {
			t.Fatalf("incomplete conversion for role %q: %+v", role, projection)
		}
		after, err := json.Marshal(content)
		if err != nil || string(before) != string(after) {
			t.Fatalf("source content changed: %v", err)
		}
	}
}

func TestTextReportsOmissions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		content *genai.Content
	}{
		{name: "nil content"},
		{name: "nil part", content: &genai.Content{Parts: []*genai.Part{nil}}},
		{name: "media", content: &genai.Content{Parts: []*genai.Part{
			{InlineData: &genai.Blob{Data: []byte("private payload"), MIMEType: "image/png"}},
		}}},
		{name: "thought", content: &genai.Content{Parts: []*genai.Part{{Thought: true, Text: "private payload"}}}},
		{name: "invalid tool data", content: &genai.Content{Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "lookup", Args: map[string]any{"value": math.NaN()}}},
		}}},
		{name: "invalid call with a result", content: &genai.Content{Parts: []*genai.Part{{
			FunctionCall:     &genai.FunctionCall{Name: "lookup", Args: map[string]any{"value": math.NaN()}},
			FunctionResponse: &genai.FunctionResponse{Name: "lookup", Response: map[string]any{"found": true}},
		}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			projection := ContentText(tc.content)
			if !projection.Incomplete || strings.Contains(projection.Text, "private payload") ||
				strings.Contains(projection.Text, "cHJpdmF0ZSBwYXlsb2Fk") {
				t.Fatalf("omitted data not handled: %+v", projection)
			}
		})
	}
}
