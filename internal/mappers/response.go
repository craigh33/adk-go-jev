package mappers

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// ToolResponse converts all response metadata and typed answers into the JSON
// object returned by an ADK tool, preserving zero probabilities and scores.
func ToolResponse(response *typesafe.Response) (map[string]any, error) {
	if response == nil {
		return nil, errors.New("systemone tool: API returned a nil response")
	}
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("systemone tool: encode response: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("systemone tool: convert response: %w", err)
	}
	return result, nil
}
