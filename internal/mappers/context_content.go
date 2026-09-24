package mappers

import (
	"encoding/json"
	"reflect"
	"strings"

	"google.golang.org/genai"
)

// ContextProjection is the text representation supplied to Jev. Unsupported
// marks content whose full meaning could not be represented.
type ContextProjection struct {
	Text        string
	Unsupported bool
}

// ContextContent projects an ADK message without modifying it or exposing opaque payloads.
func ContextContent(content *genai.Content) ContextProjection {
	if content == nil {
		return ContextProjection{Unsupported: true}
	}
	p := ContextProjection{
		Unsupported: content.Role != genai.RoleUser && content.Role != genai.RoleModel && content.Role != "",
	}
	var text strings.Builder
	text.WriteString(content.Role + ":\n")
	for _, part := range content.Parts {
		if part == nil {
			p.Unsupported = true
			continue
		}
		if !part.Thought {
			text.WriteString(part.Text)
			text.WriteByte('\n')
		}
		var data any
		if call := part.FunctionCall; call != nil {
			data = map[string]any{"call": call.Name, "id": call.ID, "arguments": call.Args}
			p.Unsupported = p.Unsupported || len(call.PartialArgs) != 0 || call.WillContinue != nil
		}
		if response := part.FunctionResponse; response != nil {
			data = map[string]any{"result": response.Name, "id": response.ID, "response": response.Response}
			p.Unsupported = p.Unsupported || len(response.Parts) != 0 || response.WillContinue != nil ||
				response.Scheduling != ""
		}
		p.Unsupported = !writeToolText(&text, data) || p.Unsupported
		remaining := *part
		remaining.Text, remaining.FunctionCall, remaining.FunctionResponse = "", nil, nil
		if !reflect.ValueOf(remaining).IsZero() {
			p.Unsupported = true
			text.WriteString("[unreviewed content retained]\n")
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
		text.WriteString("[unreviewed tool data retained]\n")
		return false
	}
	text.Write(encoded)
	text.WriteByte('\n')
	return true
}
