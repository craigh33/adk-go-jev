package contextfilter

import (
	"errors"
	"math"
	"time"

	"google.golang.org/adk/v2/agent"
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
	// Name identifies the plugin in the runner. Empty selects "context_filter".
	Name  string
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

func configure(cfg Config) (Config, error) {
	if cfg.API == nil {
		return Config{}, errors.New("contextfilter: API is required")
	}
	if math.IsNaN(cfg.RemovalThreshold) || cfg.RemovalThreshold < 0 || cfg.RemovalThreshold > 1 ||
		cfg.KeepRecentTurns < 0 || cfg.MinBytes < 0 || cfg.MaxBatchBytes < 0 || cfg.Timeout < 0 {
		return Config{}, errors.New("contextfilter: invalid threshold, turn count, byte budget or timeout")
	}
	if cfg.Name == "" {
		cfg.Name = "context_filter"
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
