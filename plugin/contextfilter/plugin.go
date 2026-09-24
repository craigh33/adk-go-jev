// Package contextfilter selects conversation turns for an ADK model request using Jev.
package contextfilter

import (
	"context"
	"errors"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
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
	report := p.evaluate(ctx, request)
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

func (p *contextFilter) evaluate(ctx agent.Context, request *model.LLMRequest) Report {
	groups := groupTurns(ctx, request.Contents, p.cfg)
	var size int
	for _, group := range groups {
		size += len(group.text)
	}
	if size < p.cfg.MinBytes {
		return Report{}
	}
	current := mappers.ContextContent(ctx.UserContent())
	if current.Unsupported {
		return Report{}
	}
	state := mappers.ContextReviewState{LatestRequest: current.Text}
	if request.Config != nil {
		state.Instructions = mappers.ContextContent(request.Config.SystemInstruction).Text
	}
	if state.LatestRequest == "" {
		return Report{}
	}
	reviewCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	return p.evaluateBatches(reviewCtx, groups, state)
}
