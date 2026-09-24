package adkcontent

import (
	"encoding/json"
	"reflect"
	"strings"

	"google.golang.org/genai"
)

// Text projects an ADK message without modifying it or exposing opaque payloads.
func Text(content *genai.Content) Projection {
	if content == nil {
		return Projection{Incomplete: true}
	}
	var p Projection
	var text strings.Builder
	text.WriteString(content.Role + ":\n")
	for _, part := range content.Parts {
		if part == nil {
			p.Incomplete = true
			continue
		}
		if !part.Thought {
			text.WriteString(part.Text)
			text.WriteByte('\n')
		}
		var data any
		if call := part.FunctionCall; call != nil {
			data = map[string]any{"call": call.Name, "id": call.ID, "arguments": call.Args}
			p.Incomplete = p.Incomplete || len(call.PartialArgs) != 0 || call.WillContinue != nil
		}
		if response := part.FunctionResponse; response != nil {
			data = map[string]any{"result": response.Name, "id": response.ID, "response": response.Response}
			p.Incomplete = p.Incomplete || len(response.Parts) != 0 || response.WillContinue != nil ||
				response.Scheduling != ""
		}
		p.Incomplete = !writeToolText(&text, data) || p.Incomplete
		remaining := *part
		remaining.Text, remaining.FunctionCall, remaining.FunctionResponse = "", nil, nil
		if !reflect.ValueOf(remaining).IsZero() {
			p.Incomplete = true
			text.WriteString("[unsupported content]\n")
		}
	}
	p.Text = text.String()
	return p
}

func writeToolText(text *strings.Builder, data any) bool {
	if data == nil {
		return true
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		text.WriteString("[unsupported tool data]\n")
		return false
	}
	text.Write(encoded)
	text.WriteByte('\n')
	return true
}
