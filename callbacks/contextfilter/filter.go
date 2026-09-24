// Package contextfilter selects conversation turns for an ADK model request using Jev.
package contextfilter

import (
	"context"
	"errors"
	"math"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

const (
	defaultMinBytes      = 8 << 10
	defaultMaxBatchBytes = 64 << 10
)

// Config controls request-only filtering. API, Pin and OnReport must be safe for
// concurrent use. Pin and OnReport must not mutate the supplied context messages.
type Config struct {
	API   typesafe.Evaluator
	Model string
	// RemovalThreshold removes turns with a relevance probability strictly below
	// this value. Zero selects 0.1; valid explicit values are greater than 0 through 1.
	RemovalThreshold float64
	// KeepRecentTurns includes the current turn. Zero selects two.
	KeepRecentTurns int
	// MinBytes skips review below this much projected conversation text. Zero selects 8 KiB.
	MinBytes int
	// MaxBatchBytes caps each JSON request. Zero selects 64 KiB. This is a byte
	// budget, not a token limit; API size rejections retain the affected turns.
	MaxBatchBytes int
	// Timeout bounds all evaluations together. Zero selects two seconds.
	// Custom evaluators must honor context cancellation.
	Timeout time.Duration
	// Pin protects the whole turn containing a selected message.
	Pin func(agent.Context, *genai.Content) bool
	// Observe reports proposed removals without modifying the request.
	Observe bool
	// OnReport receives review results, including errors that retained context.
	// It is called once per invocation of the callback, including skipped reviews.
	OnReport func(agent.Context, Report)
}

// Decision describes an assessed turn in the original request's Contents.
type Decision struct {
	Start     int // Inclusive message index.
	End       int // Exclusive message index.
	Relevance float64
	Remove    bool // Proposed removal, including in Observe mode.
}

// Report contains successful assessments and actual removals. Usage sums on-time
// Jev responses; failures may incur unreported usage. Unassessed turns are retained.
type Report struct {
	Decisions    []Decision
	RemovedTurns int
	Usage        typesafe.Usage
	Err          error
}

// New returns a callback that filters whole turns without changing saved history.
// Review errors retain the affected turns; cancellation of the parent context
// propagates. Retained messages and system instructions are not modified.
func New(cfg Config) (llmagent.BeforeModelCallback, error) {
	cfg, err := configure(cfg)
	if err != nil {
		return nil, err
	}
	return func(ctx agent.Context, request *model.LLMRequest) (*model.LLMResponse, error) {
		if request == nil {
			err := errors.New("contextfilter: model request is required")
			cfg.report(ctx, Report{Err: err})
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			cfg.report(ctx, Report{Err: err})
			return nil, err
		}
		report := cfg.review(ctx, request)
		if err := ctx.Err(); err != nil {
			report.Err = err
			cfg.report(ctx, report)
			return nil, err
		}
		if !cfg.Observe {
			request.Contents, report.RemovedTurns = selectContents(request.Contents, report.Decisions)
		}
		cfg.report(ctx, report)
		return nil, ctx.Err()
	}, nil
}

func configure(cfg Config) (Config, error) {
	if cfg.API == nil {
		return Config{}, errors.New("contextfilter: API is required")
	}
	if math.IsNaN(cfg.RemovalThreshold) || cfg.RemovalThreshold < 0 || cfg.RemovalThreshold > 1 ||
		cfg.KeepRecentTurns < 0 || cfg.MinBytes < 0 || cfg.MaxBatchBytes < 0 || cfg.Timeout < 0 {
		return Config{}, errors.New("contextfilter: invalid threshold, turn count, byte budget or timeout")
	}
	if cfg.RemovalThreshold == 0 {
		cfg.RemovalThreshold = 0.1
	}
	if cfg.KeepRecentTurns == 0 {
		cfg.KeepRecentTurns = 2
	}
	if cfg.MinBytes == 0 {
		cfg.MinBytes = defaultMinBytes
	}
	if cfg.MaxBatchBytes == 0 {
		cfg.MaxBatchBytes = defaultMaxBatchBytes
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Second
	}
	return cfg, nil
}

func (cfg Config) report(ctx agent.Context, report Report) {
	if cfg.OnReport != nil {
		cfg.OnReport(ctx, report)
	}
}

func selectContents(contents []*genai.Content, decisions []Decision) ([]*genai.Content, int) {
	var selected []*genai.Content
	removed, start := 0, 0
	for _, decision := range decisions {
		if decision.Remove {
			selected = append(selected, contents[start:decision.Start]...)
			start = decision.End
			removed++
		}
	}
	if removed == 0 {
		return contents, 0
	}
	return append(selected, contents[start:]...), removed
}

func (cfg Config) review(ctx agent.Context, request *model.LLMRequest) Report {
	groups := groupContents(ctx, request.Contents, cfg)
	var size int
	for _, group := range groups {
		size += len(group.text)
	}
	if size < cfg.MinBytes {
		return Report{}
	}
	current := projectContent(ctx.UserContent())
	if current.unsafe {
		return Report{}
	}
	state := reviewState{LatestRequest: current.text}
	if request.Config != nil {
		state.Instructions = projectContent(request.Config.SystemInstruction).text
	}
	if state.LatestRequest == "" {
		return Report{}
	}
	reviewCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return cfg.evaluateBatches(reviewCtx, groups, state)
}
