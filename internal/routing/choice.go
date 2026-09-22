// Package routing shares confidence decisions between ADK routing and evaluations.
package routing

import (
	"errors"
	"fmt"
	"math"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// ValidateThreshold checks a caller-supplied confidence threshold.
func ValidateThreshold(value float64) error {
	if math.IsNaN(value) || value < 0 || value > 1 {
		return errors.New("minimum confidence must be between zero and one")
	}
	return nil
}

// Choice returns the answer and whether it meets the minimum confidence.
func Choice(response *typesafe.Response, name string, minimum float64) (typesafe.ChoiceAnswer, bool, error) {
	if response == nil {
		return typesafe.ChoiceAnswer{}, false, errors.New("API returned a nil response")
	}
	answer, ok := response.Answers[name].(typesafe.ChoiceAnswer)
	if !ok {
		return typesafe.ChoiceAnswer{}, false, fmt.Errorf("answer %q must be a ChoiceAnswer", name)
	}
	if err := ValidateThreshold(answer.Confidence); err != nil {
		return typesafe.ChoiceAnswer{}, false, err
	}
	return answer, answer.Confidence >= minimum, nil
}
