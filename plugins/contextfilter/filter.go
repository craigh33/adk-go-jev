package contextfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func (p *contextFilter) filter(ctx context.Context, conversation *conversation) Report {
	var size int
	for _, turn := range conversation.turns {
		size += len(turn.text)
	}
	if size < p.cfg.MinBytes || !slices.ContainsFunc(conversation.turns, func(turn turn) bool { return !turn.pinned }) {
		return Report{}
	}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	batches, err := p.packBatches(ctx, conversation)
	report := Report{Err: err}
	for _, batch := range batches {
		response, err := p.evaluate(ctx, batch.request)
		if response != nil {
			report.Usage.InputTokens += response.Usage.InputTokens
			report.Usage.OutputTokens += response.Usage.OutputTokens
		}
		if err != nil {
			report.Err = errors.Join(
				report.Err,
				fmt.Errorf("contextfilter: review at %d: %w", batch.turns[0].start, err),
			)
			break
		}
		p.collectDecisions(&report, batch.turns, response)
	}
	return report
}

func (p *contextFilter) packBatches(ctx context.Context, conversation *conversation) ([]batch, error) {
	base, err := p.newBatch(conversation, nil)
	if err != nil {
		return nil, err
	}
	if base.size > p.cfg.MaxRequestBytes {
		return nil, errors.New("contextfilter: protected context exceeds request byte limit")
	}
	var batches []batch
	var packingErr error
	pending := base
	for _, candidate := range conversation.turns {
		if err := ctx.Err(); err != nil {
			return batches, errors.Join(packingErr, err)
		}
		if candidate.pinned {
			continue
		}
		next, err := p.newBatch(conversation, append(slices.Clone(pending.turns), candidate))
		if err != nil {
			return batches, errors.Join(packingErr, err)
		}
		if next.size > p.cfg.MaxRequestBytes && len(pending.turns) > 0 {
			batches = append(batches, pending)
			pending = base
			next, err = p.newBatch(conversation, []turn{candidate})
			if err != nil {
				return batches, errors.Join(packingErr, err)
			}
		}
		if next.size > p.cfg.MaxRequestBytes {
			packingErr = errors.Join(
				packingErr,
				fmt.Errorf("contextfilter: turn at %d exceeds request byte limit", candidate.start),
			)
			continue
		}
		pending = next
	}
	if len(pending.turns) > 0 {
		batches = append(batches, pending)
	}
	return batches, packingErr
}

func (p *contextFilter) newBatch(conversation *conversation, turns []turn) (batch, error) {
	state := reviewState{LatestRequest: conversation.latestRequest, Instructions: conversation.instructions}
	questions := make(map[string]typesafe.Question, len(turns))
	for _, turn := range turns {
		id := strconv.Itoa(turn.start)
		questions[id] = typesafe.Noul{
			Instructions: fmt.Sprintf(relevanceInstructions, id),
			Criteria:     &typesafe.NoulCriteria{True: relevantCriteria, False: irrelevantCriteria},
		}
	}
	for _, turn := range conversation.turns {
		id := strconv.Itoa(turn.start)
		if turn.pinned || questions[id] != nil {
			state.Groups = append(state.Groups, reviewGroup{ID: id, Pinned: turn.pinned, Text: turn.text})
		}
	}
	request := &typesafe.Request{Model: p.cfg.Model, State: state, Questions: questions}
	data, err := json.Marshal(request)
	if err != nil {
		return batch{}, err
	}
	return batch{request: request, turns: turns, size: len(data)}, nil
}

func (p *contextFilter) evaluate(ctx context.Context, request *typesafe.Request) (*typesafe.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response, err := p.cfg.API.Evaluate(ctx, request)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return response, err
	}
	if response == nil {
		return nil, errors.New("contextfilter: missing evaluation response")
	}
	return response, nil
}

func (p *contextFilter) collectDecisions(report *Report, turns []turn, response *typesafe.Response) {
	for _, turn := range turns {
		id := strconv.Itoa(turn.start)
		relevance, err := mappers.NoulProbability(response.Answers[id])
		if err != nil {
			report.Err = errors.Join(report.Err, fmt.Errorf("contextfilter: turn at %s: %w", id, err))
			continue
		}
		report.Decisions = append(report.Decisions, Decision{
			Start: turn.start, End: turn.end, Relevance: relevance, Remove: relevance < p.cfg.RemovalThreshold,
		})
	}
}
