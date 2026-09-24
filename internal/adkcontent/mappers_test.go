package adkcontent

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"google.golang.org/genai"
)

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
		}}
		before, err := json.Marshal(content)
		if err != nil {
			t.Fatal(err)
		}
		projection := Text(content)
		if projection.Incomplete || !strings.HasPrefix(projection.Text, role+":\nFind the record\n") ||
			!strings.Contains(projection.Text, `"arguments":{"id":1}`) ||
			!strings.Contains(projection.Text, `"response":{"found":true}`) {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			projection := Text(tc.content)
			if !projection.Incomplete || strings.Contains(projection.Text, "private payload") ||
				strings.Contains(projection.Text, "cHJpdmF0ZSBwYXlsb2Fk") {
				t.Fatalf("omitted data not handled: %+v", projection)
			}
		})
	}
}
