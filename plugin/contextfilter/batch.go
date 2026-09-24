package contextfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func (p *contextFilter) batchRequest(
	turns []turn,
	candidates []int,
	state mappers.ContextReviewState,
) *typesafe.Request {
	selected := make(map[int]bool, len(candidates))
	for _, i := range candidates {
		selected[i] = true
	}
	for i, turn := range turns {
		if turn.pinned || selected[i] {
			state.Groups = append(state.Groups, mappers.ContextReviewGroup{
				ID: strconv.Itoa(turn.start), Pinned: turn.pinned, Text: turn.text,
			})
		}
	}
	return mappers.ContextReviewRequest(p.cfg.Model, state)
}

func (p *contextFilter) evaluateBatches(ctx context.Context, groups []turn, state mappers.ContextReviewState) Report {
	var report Report
	var batch []int
	for i, group := range groups {
		if group.pinned {
			continue
		}
		if err := ctx.Err(); err != nil {
			report.Err = errors.Join(report.Err, err)
			break
		}
		batch = append(batch, i)
		trial := batch
		if !p.batchFits(groups, trial, state) {
			if len(batch) > 1 {
				p.evaluateBatch(ctx, groups, batch[:len(batch)-1], state, &report)
			}
			batch = nil
			trial = []int{i}
			if !p.batchFits(groups, trial, state) {
				report.Err = errors.Join(
					report.Err,
					fmt.Errorf("contextfilter: turn at %d exceeds review byte budget", group.start),
				)
				continue
			}
		}
		batch = trial
	}
	if len(batch) != 0 && ctx.Err() == nil {
		p.evaluateBatch(ctx, groups, batch, state, &report)
	}
	return report
}

func (p *contextFilter) batchFits(groups []turn, candidates []int, state mappers.ContextReviewState) bool {
	data, err := json.Marshal(p.batchRequest(groups, candidates, state))
	return err == nil && len(data) <= p.cfg.MaxBatchBytes
}

func (p *contextFilter) evaluateBatch(
	ctx context.Context,
	groups []turn,
	candidates []int,
	state mappers.ContextReviewState,
	report *Report,
) {
	if err := ctx.Err(); err != nil {
		report.Err = errors.Join(report.Err, err)
		return
	}
	response, err := p.cfg.API.Evaluate(ctx, p.batchRequest(groups, candidates, state))
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		report.Err = errors.Join(report.Err, err)
		return
	}
	if response == nil {
		report.Err = errors.Join(report.Err, errors.New("contextfilter: missing evaluation response"))
		return
	}
	report.Usage.InputTokens += response.Usage.InputTokens
	report.Usage.OutputTokens += response.Usage.OutputTokens
	for _, i := range candidates {
		group := groups[i]
		id := strconv.Itoa(group.start)
		relevance, err := mappers.ContextRelevance(response.Answers[id], id)
		if err != nil {
			report.Err = errors.Join(report.Err, err)
			continue
		}
		report.Decisions = append(report.Decisions, Decision{
			Start: group.start, End: group.end, Relevance: relevance, Remove: relevance < p.cfg.RemovalThreshold,
		})
	}
}
