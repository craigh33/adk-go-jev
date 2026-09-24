// Package mappers converts ADK content and TypeSafe responses without applying policy.
package mappers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// ResponseMap converts response metadata and typed answers into a JSON-compatible
// map, preserving zero probabilities and scores.
func ResponseMap(response *typesafe.Response) (map[string]any, error) {
	if response == nil {
		return nil, errors.New("response is nil")
	}
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode response: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("convert response: %w", err)
	}
	return result, nil
}

// NoulProbability extracts a finite probability between zero and one from a Noul answer.
func NoulProbability(answer typesafe.Answer) (float64, error) {
	value, ok := answer.(typesafe.NoulAnswer)
	if !ok || math.IsNaN(value.Noul) || value.Noul < 0 || value.Noul > 1 {
		return 0, errors.New("answer must contain a Noul probability between zero and one")
	}
	return value.Noul, nil
}

// ContentText projects an ADK message without modifying it or exposing opaque payloads.
func ContentText(content *genai.Content) Projection {
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
