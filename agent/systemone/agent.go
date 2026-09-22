// Package systemone provides classification and routing agents for ADK.
package systemone

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"math"
	"slices"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// Config configures a standalone classifier, optionally followed by a child agent.
// Questions and nested values must not be mutated after construction.
type Config struct {
	Name        string
	Description string
	API         typesafe.Evaluator
	Model       string
	Questions   map[string]typesafe.Question
	// State defaults to the initiating user message's text. Override for structured state.
	State func(agent.InvocationContext) (any, error)
	// OutputKey stores the assessment in session state; defaults to Name.
	OutputKey string
	Routing   *Routing
}

// Routing selects a child from a Choice answer. Low confidence or an unmapped
// choice selects Fallback. API errors stop execution instead of selecting a child.
type Routing struct {
	Question      string
	MinConfidence float64
	Routes        map[string]agent.Agent
	Fallback      agent.Agent
}

// New creates an ADK custom agent. It emits the assessment and stores it in
// session state before running the selected child. No chat model is needed
// for classification. Consumers receive events from both classifier and child.
func New(cfg Config) (agent.Agent, error) {
	if strings.TrimSpace(cfg.Name) == "" || cfg.Name == "user" {
		return nil, errors.New("systemone agent: a non-reserved name is required")
	}
	if cfg.API == nil {
		return nil, errors.New("systemone agent: API is required")
	}
	cfg.Questions = maps.Clone(cfg.Questions)
	if err := (&typesafe.Request{State: "", Model: cfg.Model, Questions: cfg.Questions}).Validate(); err != nil {
		return nil, err
	}
	if cfg.State == nil {
		cfg.State = userText
	}
	if cfg.OutputKey == "" {
		cfg.OutputKey = cfg.Name
	}
	var children []agent.Agent
	if cfg.Routing != nil {
		copyRouting := *cfg.Routing
		copyRouting.Routes = maps.Clone(copyRouting.Routes)
		cfg.Routing = &copyRouting
		var err error
		children, err = routingAgents(cfg)
		if err != nil {
			return nil, err
		}
	}
	return agent.New(agent.Config{Name: cfg.Name, Description: cfg.Description, SubAgents: children, Run: cfg.run})
}

func (cfg Config) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		response, err := cfg.evaluate(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		record, err := mappers.ToolResponse(response)
		if err != nil {
			yield(nil, err)
			return
		}
		var selected agent.Agent
		if cfg.Routing != nil {
			selected, err = cfg.Routing.selectAgent(response, record)
			if err != nil {
				yield(nil, err)
				return
			}
		}
		data, err := json.Marshal(record)
		if err != nil {
			yield(nil, err)
			return
		}
		event := session.NewEvent(ctx, ctx.InvocationID())
		event.Author, event.Branch, event.IsolationScope = cfg.Name, ctx.Branch(), ctx.IsolationScope()
		event.Content = genai.NewContentFromText(string(data), genai.RoleModel)
		event.Actions.StateDelta[cfg.OutputKey] = record
		if !yield(event, nil) || ctx.Ended() || selected == nil {
			return
		}
		for event, err := range selected.Run(ctx) {
			if !yield(event, err) || err != nil {
				return
			}
		}
	}
}

func (cfg Config) evaluate(ctx agent.InvocationContext) (*typesafe.Response, error) {
	state, err := cfg.State(ctx)
	if err != nil {
		return nil, err
	}
	return cfg.API.Evaluate(ctx, &typesafe.Request{State: state, Model: cfg.Model, Questions: cfg.Questions})
}

func (r *Routing) selectAgent(response *typesafe.Response, record map[string]any) (agent.Agent, error) {
	if response == nil {
		return nil, errors.New("API returned a nil response")
	}
	answer, ok := response.Answers[r.Question].(typesafe.ChoiceAnswer)
	if !ok {
		return nil, fmt.Errorf("answer %q must be a ChoiceAnswer", r.Question)
	}
	if err := validateConfidence(answer.Confidence); err != nil {
		return nil, err
	}
	selected := r.Routes[answer.Choice]
	fallback := answer.Confidence < r.MinConfidence || selected == nil
	if fallback {
		selected = r.Fallback
	}
	record["routing"] = map[string]any{
		"agent": selected.Name(), "fallback": fallback, "choice": answer.Choice, "confidence": answer.Confidence,
	}
	return selected, nil
}

func validateConfidence(value float64) error {
	if math.IsNaN(value) || value < 0 || value > 1 {
		return errors.New("confidence must be between zero and one")
	}
	return nil
}

func userText(ctx agent.InvocationContext) (any, error) {
	var text strings.Builder
	if content := ctx.UserContent(); content != nil {
		for _, part := range content.Parts {
			if part != nil && !part.Thought && part.Text != "" {
				if text.Len() > 0 {
					text.WriteByte('\n')
				}
				text.WriteString(part.Text)
			}
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return nil, errors.New("systemone agent: user text is required; configure State for structured input")
	}
	return text.String(), nil
}

func routingAgents(cfg Config) ([]agent.Agent, error) {
	route := cfg.Routing
	if err := validateConfidence(route.MinConfidence); err != nil {
		return nil, err
	}
	var criteria map[string]any
	switch q := cfg.Questions[route.Question].(type) {
	case typesafe.Choice:
		criteria = q.Criteria
	case *typesafe.Choice:
		criteria = q.Criteria
	default:
		return nil, errors.New("systemone agent: routing requires a configured Choice question")
	}
	if route.Fallback == nil || len(route.Routes) == 0 {
		return nil, errors.New("systemone agent: routes and a fallback agent are required")
	}
	children := []agent.Agent{route.Fallback}
	names := map[string]agent.Agent{route.Fallback.Name(): route.Fallback}
	for _, label := range slices.Sorted(maps.Keys(route.Routes)) {
		child := route.Routes[label]
		if _, ok := criteria[label]; !ok {
			return nil, fmt.Errorf("systemone agent: route %q is not a configured choice", label)
		}
		if child == nil {
			return nil, fmt.Errorf("systemone agent: route %q has no agent", label)
		}
		if previous, exists := names[child.Name()]; exists {
			if previous != child {
				return nil, fmt.Errorf("systemone agent: duplicate child name %q", child.Name())
			}
			continue
		}
		names[child.Name()] = child
		children = append(children, child)
	}
	for _, child := range children {
		if strings.TrimSpace(child.Name()) == "" || child.Name() == cfg.Name || child.Name() == "user" {
			return nil, errors.New("systemone agent: child names must be non-reserved and distinct from the classifier")
		}
	}
	return children, nil
}
