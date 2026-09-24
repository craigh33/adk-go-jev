package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ResponseMap converts response metadata and typed answers into a JSON-compatible
// map, preserving zero probabilities and scores.
func ResponseMap(response *Response) (map[string]any, error) {
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
func NoulProbability(answer Answer) (float64, error) {
	value, ok := answer.(NoulAnswer)
	if !ok || math.IsNaN(value.Noul) || value.Noul < 0 || value.Noul > 1 {
		return 0, errors.New("answer must contain a Noul probability between zero and one")
	}
	return value.Noul, nil
}
