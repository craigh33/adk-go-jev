package systemone

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// EvaluationAPI is the portion of the TypeSafe client used by the tool.
type EvaluationAPI = typesafe.Evaluator

// Config defines an ADK evaluation tool. Questions and rubrics are set by the
// application; the calling model supplies only the state to evaluate.
type Config struct {
	API         EvaluationAPI
	Name        string
	Description string
	Model       string
	// Questions must not be mutated after construction, including nested values.
	Questions map[string]typesafe.Question
}

// Input is the text submitted by the calling agent. For structured Go state,
// call typesafe.Client.Evaluate directly.
type Input struct {
	State string `json:"state" jsonschema:"Text to evaluate against the configured questions"`
}

// New creates an ADK function tool with application-defined questions.
// Name defaults to evaluate_systemone. Model defaults to the API client's model.
func New(cfg Config) (tool.Tool, error) {
	if cfg.API == nil {
		return nil, errors.New("systemone tool: API is required")
	}
	request := typesafe.Request{State: "", Model: cfg.Model, Questions: maps.Clone(cfg.Questions)}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("systemone tool: %w", err)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "evaluate_systemone"
	}
	if cfg.Description == "" {
		cfg.Description = "Evaluate text using predefined System One questions and return answers with probabilities and confidence."
	}
	return functiontool.New(functiontool.Config{
		Name: cfg.Name, Description: cfg.Description,
	}, func(ctx agent.Context, input Input) (map[string]any, error) {
		call := request
		call.State = input.State
		response, err := cfg.API.Evaluate(ctx, &call)
		if err != nil {
			return nil, fmt.Errorf("systemone tool: evaluate: %w", err)
		}
		return mappers.ToolResponse(response)
	})
}
