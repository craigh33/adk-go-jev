// Package contextfilter selects conversation turns for an ADK model request using Jev.
package contextfilter

import (
	"errors"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"
)

// New returns an ADK plugin that filters whole turns without changing saved history.
// Evaluation errors retain the affected turns; parent cancellation propagates.
func New(cfg Config) (*plugin.Plugin, error) {
	cfg, err := cfg.configure()
	if err != nil {
		return nil, err
	}
	p := &contextFilter{cfg: cfg}
	return plugin.New(plugin.Config{Name: cfg.Name, BeforeModelCallback: p.beforeModel})
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
	conversation := p.newConversation(ctx, request)
	report := p.filter(ctx, &conversation)
	if err := ctx.Err(); err != nil {
		report.Err = errors.Join(report.Err, err)
		p.report(ctx, report)
		return nil, err
	}
	if !p.cfg.Observe {
		request.Contents, report.RemovedTurns = conversation.selectTurns(report.Decisions)
	}
	p.report(ctx, report)
	return nil, ctx.Err()
}

func (p *contextFilter) report(ctx agent.Context, report Report) {
	if p.cfg.OnReport != nil {
		p.cfg.OnReport(ctx, report)
	}
}
