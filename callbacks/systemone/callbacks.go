// Package systemone assesses model inputs, outputs, and tool calls through ADK callbacks.
package systemone

import (
	"errors"
	"fmt"
	"maps"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// Decision is the application's policy result. Reason is returned when Block is true.
type Decision struct {
	Block  bool   `json:"block"`
	Reason string `json:"reason,omitempty"`
}

// Config fixes the questions and requires an explicit application policy.
// Questions, nested values, and Policy must be safe for concurrent use.
// API, policy, or state-storage errors propagate; they never implicitly allow execution.
type Config struct {
	API       typesafe.Evaluator
	Model     string
	Questions map[string]typesafe.Question
	Policy    func(agent.Context, *typesafe.Response) (Decision, error)
	// OutputKey optionally stores the assessment and decision in session state.
	OutputKey string
}

// BeforeModel assesses structured request messages before the model is called.
func BeforeModel(cfg Config) (llmagent.BeforeModelCallback, error) {
	cfg, err := configure(cfg)
	if err != nil {
		return nil, err
	}
	return func(ctx agent.Context, request *model.LLMRequest) (*model.LLMResponse, error) {
		if request == nil {
			return nil, errors.New("systemone callback: missing model request")
		}
		decision, err := cfg.assess(ctx, map[string]any{"messages": request.Contents})
		if err != nil || !decision.Block {
			return nil, err
		}
		return blockedResponse(decision.Reason), nil
	}, nil
}

// AfterModel assesses complete output, replacing it when the policy blocks.
// Use non-streaming execution: partial chunks are rejected to prevent forwarding
// output before it has been assessed. Existing model errors are preserved.
func AfterModel(cfg Config) (llmagent.AfterModelCallback, error) {
	cfg, err := configure(cfg)
	if err != nil {
		return nil, err
	}
	return func(ctx agent.Context, response *model.LLMResponse, modelErr error) (*model.LLMResponse, error) {
		if modelErr != nil {
			return nil, modelErr
		}
		if response == nil {
			return nil, errors.New("systemone callback: missing model response")
		}
		if response.Partial {
			return nil, errors.New("systemone callback: output assessment requires non-streaming responses")
		}
		decision, err := cfg.assess(ctx, map[string]any{"response": response.Content})
		if err != nil || !decision.Block {
			return nil, err
		}
		return blockedResponse(decision.Reason), nil
	}, nil
}

// BeforeTool assesses a tool's name and arguments. A blocked result prevents
// execution and returns a structured explanation as the tool result.
func BeforeTool(cfg Config) (llmagent.BeforeToolCallback, error) {
	cfg, err := configure(cfg)
	if err != nil {
		return nil, err
	}
	return func(ctx agent.Context, target tool.Tool, args map[string]any) (map[string]any, error) {
		if target == nil {
			return nil, errors.New("systemone callback: missing tool")
		}
		decision, err := cfg.assess(ctx, map[string]any{"tool": target.Name(), "arguments": args})
		if err != nil || !decision.Block {
			return nil, err
		}
		return map[string]any{"blocked": true, "reason": decision.Reason}, nil
	}, nil
}

func configure(cfg Config) (Config, error) {
	if cfg.API == nil || cfg.Policy == nil {
		return Config{}, errors.New("systemone callback: API and Policy are required")
	}
	cfg.Questions = maps.Clone(cfg.Questions)
	if err := (&typesafe.Request{State: "", Model: cfg.Model, Questions: cfg.Questions}).Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) assess(ctx agent.Context, state any) (Decision, error) {
	response, err := cfg.API.Evaluate(ctx, &typesafe.Request{State: state, Model: cfg.Model, Questions: cfg.Questions})
	if err != nil {
		return Decision{}, fmt.Errorf("systemone callback: evaluate: %w", err)
	}
	if response == nil {
		return Decision{}, errors.New("systemone callback: API returned nil response")
	}
	decision, err := cfg.Policy(ctx, response)
	if err != nil {
		return Decision{}, fmt.Errorf("systemone callback: policy: %w", err)
	}
	if decision.Block && decision.Reason == "" {
		decision.Reason = "The configured assessment policy requires review."
	}
	if cfg.OutputKey != "" {
		record, err := typesafe.ResponseMap(response)
		if err != nil {
			return Decision{}, err
		}
		record["decision"] = map[string]any{"block": decision.Block, "reason": decision.Reason}
		if err := ctx.State().Set(cfg.OutputKey, record); err != nil {
			return Decision{}, fmt.Errorf("systemone callback: save assessment: %w", err)
		}
	}
	return decision, nil
}

func blockedResponse(reason string) *model.LLMResponse {
	return &model.LLMResponse{Content: genai.NewContentFromText(reason, genai.RoleModel), TurnComplete: true}
}
