package typesafe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apitypes "github.com/craigh33/adk-go-typesafe/internal/typesafe"
)

const (
	maxChoices = 255
	minLevels  = 1
	maxLevels  = 10
)

// Request evaluates named questions against text or structured JSON state.
// An empty Model uses the client's default. Do not mutate a request while it is
// being evaluated.
type Request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// MarshalJSON maps the ergonomic question interface to the generated wire request.
func (r *Request) MarshalJSON() ([]byte, error) {
	questions := make(map[string]apitypes.Question, len(r.Questions))
	for name, question := range r.Questions {
		data, err := json.Marshal(question)
		if err != nil {
			return nil, fmt.Errorf("question %q: %w", name, err)
		}
		var wire apitypes.Question
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		questions[name] = wire
	}
	return json.Marshal(apitypes.SystemOneRequest{State: r.State, Model: r.Model, Questions: questions})
}

// Validate checks the request's JSON shapes and documented rubric limits.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("typesafe: request is required")
	}
	if err := validateContent(r.State, false); err != nil {
		return fmt.Errorf("typesafe: state: %w", err)
	}
	if len(r.Questions) == 0 {
		return errors.New("typesafe: questions must not be empty")
	}
	for name, question := range r.Questions {
		if strings.TrimSpace(name) == "" {
			return errors.New("typesafe: question name must not be blank")
		}
		if err := validateQuestion(question); err != nil {
			return fmt.Errorf("typesafe: question %q: %w", name, err)
		}
	}
	return nil
}

func validateContent(value any, nullable bool) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = bytes.TrimSpace(data)
	if nullable && bytes.Equal(data, []byte("null")) {
		return nil
	}
	if len(data) != 0 && (data[0] == '"' || data[0] == '{' || data[0] == '[') {
		return nil
	}
	return errors.New("expected a string, object, or array")
}

func validateQuestion(question Question) error {
	data, err := json.Marshal(question)
	if err != nil {
		return fmt.Errorf("encode question: %w", err)
	}
	var wire struct {
		Type         string          `json:"type"`
		Instructions json.RawMessage `json:"instructions"`
		Criteria     json.RawMessage `json:"criteria"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode question: %w", err)
	}
	if err := validateContent(wire.Instructions, true); err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	switch wire.Type {
	case choiceType:
		return validateChoiceCriteria(wire.Criteria)
	case scoreType:
		return validateScoreCriteria(wire.Criteria)
	case noulType:
		return validateNoulCriteria(wire.Criteria)
	default:
		return errors.New("question must be a non-nil Choice, Score, or Noul")
	}
}

func validateChoiceCriteria(data json.RawMessage) error {
	var criteria map[string]json.RawMessage
	if err := json.Unmarshal(data, &criteria); err != nil {
		return fmt.Errorf("choice criteria: %w", err)
	}
	if len(criteria) == 0 || len(criteria) > maxChoices {
		return fmt.Errorf("choice requires 1 to %d options", maxChoices)
	}
	for _, value := range criteria {
		if err := validateContent(value, true); err != nil {
			return fmt.Errorf("choice criteria: %w", err)
		}
	}
	return nil
}

func validateScoreCriteria(data json.RawMessage) error {
	var criteria []json.RawMessage
	if err := json.Unmarshal(data, &criteria); err != nil {
		return fmt.Errorf("score criteria: %w", err)
	}
	if len(criteria) < minLevels || len(criteria) > maxLevels {
		return fmt.Errorf("score requires %d to %d levels", minLevels, maxLevels)
	}
	for _, value := range criteria {
		if err := validateContent(value, false); err != nil {
			return fmt.Errorf("score criteria: %w", err)
		}
	}
	return nil
}

func validateNoulCriteria(data json.RawMessage) error {
	if len(data) == 0 {
		return nil
	}
	var criteria NoulCriteria
	if err := json.Unmarshal(data, &criteria); err != nil {
		return fmt.Errorf("noul criteria: %w", err)
	}
	for _, value := range []any{criteria.True, criteria.False} {
		if err := validateContent(value, true); err != nil {
			return fmt.Errorf("noul criteria: %w", err)
		}
	}
	return nil
}
