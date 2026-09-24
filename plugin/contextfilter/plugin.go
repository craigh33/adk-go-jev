// Package contextfilter selects conversation turns for an ADK model request using Jev.
package contextfilter

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// New returns an ADK plugin that filters whole turns without changing saved history.
// Evaluation errors retain the affected turns; parent cancellation propagates.
func New(cfg Config) (*plugin.Plugin, error) {
	cfg, err := configure(cfg)
	if err != nil {
		return nil, err
	}
	p := &contextFilter{cfg: cfg}
	return plugin.New(plugin.Config{Name: cfg.Name, BeforeModelCallback: p.beforeModel})
}

type contextFilter struct {
	cfg Config
}

func (p *contextFilter) beforeModel(ctx agent.Context, request *model.LLMRequest) (*model.LLMResponse, error) {
	if request == nil {
		err := errors.New("contextfilter: model request is required")
		p.report(ctx, Report{Err: err})
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		p.report(ctx, Report{Err: err})
		return nil, err
	}
	report := p.assessTurns(ctx, request)
	if err := ctx.Err(); err != nil {
		report.Err = err
		p.report(ctx, report)
		return nil, err
	}
	if !p.cfg.Observe {
		request.Contents, report.RemovedTurns = selectTurns(request.Contents, report.Decisions)
	}
	p.report(ctx, report)
	return nil, ctx.Err()
}

func (p *contextFilter) report(ctx agent.Context, report Report) {
	if p.cfg.OnReport != nil {
		p.cfg.OnReport(ctx, report)
	}
}

func (p *contextFilter) assessTurns(ctx agent.Context, request *model.LLMRequest) Report {
	current := mappers.ContextContent(ctx.UserContent())
	if current.Unsupported {
		return Report{}
	}
	turns := groupTurns(ctx, request.Contents, p.cfg)
	var size int
	for _, turn := range turns {
		size += len(turn.text)
	}
	if size < p.cfg.MinBytes {
		return Report{}
	}
	evaluation := p.newRequest(request, turns, current.Text)
	if len(evaluation.Questions) == 0 {
		return Report{}
	}
	response, err := p.evaluate(ctx, evaluation)
	if err != nil {
		return Report{Err: err}
	}
	report := Report{Usage: response.Usage}
	for _, turn := range turns {
		if turn.pinned {
			continue
		}
		id := strconv.Itoa(turn.start)
		relevance, err := mappers.ContextRelevance(response.Answers[id], id)
		if err != nil {
			report.Err = errors.Join(report.Err, err)
			continue
		}
		report.Decisions = append(report.Decisions, Decision{
			Start: turn.start, End: turn.end, Relevance: relevance, Remove: relevance < p.cfg.RemovalThreshold,
		})
	}
	return report
}

func (p *contextFilter) evaluate(ctx context.Context, request *typesafe.Request) (*typesafe.Response, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(data) > p.cfg.MaxRequestBytes {
		return nil, errors.New("contextfilter: evaluation exceeds request byte limit")
	}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	response, err := p.cfg.API.Evaluate(ctx, request)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("contextfilter: missing evaluation response")
	}
	return response, nil
}
