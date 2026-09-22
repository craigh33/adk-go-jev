// Package evaluation measures Choice accuracy, fallback rates, latency, and token usage.
package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/craigh33/adk-go-typesafe/internal/routing"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// Dataset applies one Choice rubric and confidence threshold to labelled cases.
type Dataset struct {
	Model         string          `json:"model,omitempty"`
	Question      typesafe.Choice `json:"question"`
	MinConfidence float64         `json:"min_confidence"`
	Cases         []Case          `json:"cases"`
}

// Case expects either a choice or a fallback, never both.
type Case struct {
	Name             string `json:"name"`
	State            any    `json:"state"`
	ExpectedChoice   string `json:"expected_choice,omitempty"`
	ExpectedFallback bool   `json:"expected_fallback,omitempty"`
}

// Result records one evaluation without copying the submitted state.
type Result struct {
	Name             string         `json:"name"`
	ExpectedChoice   string         `json:"expected_choice,omitempty"`
	ExpectedFallback bool           `json:"expected_fallback"`
	Choice           string         `json:"choice,omitempty"`
	Confidence       float64        `json:"confidence"`
	Fallback         bool           `json:"fallback"`
	Correct          bool           `json:"correct"`
	LatencyMS        float64        `json:"latency_ms"`
	Model            string         `json:"model,omitempty"`
	Usage            typesafe.Usage `json:"usage"`
	Error            string         `json:"error,omitempty"`
}

// Report summarizes processed cases. Accuracy includes expected fallbacks and
// counts errors as incorrect; SelectedAccuracy covers only confident choices.
// FallbackRate and Coverage use all processed cases as the denominator. Latency
// includes failed calls and uses nearest-rank percentiles. Usage sums successful
// evaluations; failed calls and retries may incur additional unreported usage.
type Report struct {
	Total            int            `json:"total"`
	Correct          int            `json:"correct"`
	Selected         int            `json:"selected"`
	Fallbacks        int            `json:"fallbacks"`
	Errors           int            `json:"errors"`
	Accuracy         float64        `json:"accuracy"`
	SelectedAccuracy float64        `json:"selected_accuracy"`
	FallbackRate     float64        `json:"fallback_rate"`
	Coverage         float64        `json:"coverage"`
	P50LatencyMS     float64        `json:"p50_latency_ms"`
	P95LatencyMS     float64        `json:"p95_latency_ms"`
	Usage            typesafe.Usage `json:"usage"`
	Models           []string       `json:"models"`
	Results          []Result       `json:"results"`
}

// Load reads exactly one dataset, rejecting unknown fields and invalid rubrics.
func Load(r io.Reader) (*Dataset, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var dataset Dataset
	if err := decoder.Decode(&dataset); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("evaluation: expected one JSON dataset")
	}
	if err := dataset.Validate(); err != nil {
		return nil, err
	}
	return &dataset, nil
}

// Validate checks all cases before any request is sent.
func (d Dataset) Validate() error {
	if err := routing.ValidateThreshold(d.MinConfidence); err != nil {
		return err
	}
	if len(d.Cases) == 0 {
		return errors.New("evaluation: at least one case is required")
	}
	names := make(map[string]bool, len(d.Cases))
	for _, test := range d.Cases {
		if strings.TrimSpace(test.Name) == "" || names[test.Name] {
			return errors.New("evaluation: case names must be non-blank and unique")
		}
		names[test.Name] = true
		if test.ExpectedFallback {
			if test.ExpectedChoice != "" {
				return fmt.Errorf("evaluation: case %q expects both a choice and fallback", test.Name)
			}
		} else if _, ok := d.Question.Criteria[test.ExpectedChoice]; !ok || test.ExpectedChoice == "" {
			return fmt.Errorf("evaluation: case %q must expect a configured choice or fallback", test.Name)
		}
		if err := (&typesafe.Request{State: test.State, Model: d.Model, Questions: map[string]typesafe.Question{"route": d.Question}}).Validate(); err != nil {
			return fmt.Errorf("evaluation: case %q: %w", test.Name, err)
		}
	}
	return nil
}

// Run evaluates cases sequentially. Per-case API failures are recorded and do
// not stop later cases. Cancellation returns the partial report and context error.
// The dataset must not be mutated while Run is executing.
func Run(ctx context.Context, api typesafe.Evaluator, dataset Dataset) (*Report, error) {
	if api == nil {
		return nil, errors.New("evaluation: API is required")
	}
	if err := dataset.Validate(); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(dataset.Cases))
	for _, test := range dataset.Cases {
		if err := ctx.Err(); err != nil {
			return summarize(results), err
		}
		started := time.Now()
		response, err := api.Evaluate(
			ctx,
			&typesafe.Request{
				State:     test.State,
				Model:     dataset.Model,
				Questions: map[string]typesafe.Question{"route": dataset.Question},
			},
		)
		result := Result{
			Name:             test.Name,
			ExpectedChoice:   test.ExpectedChoice,
			ExpectedFallback: test.ExpectedFallback,
			LatencyMS:        float64(time.Since(started)) / float64(time.Millisecond),
		}
		if err != nil {
			result.Error = err.Error()
		} else {
			assessResult(&result, response, dataset)
		}
		results = append(results, result)
		if err := ctx.Err(); err != nil {
			return summarize(results), err
		}
	}
	return summarize(results), nil
}

func assessResult(result *Result, response *typesafe.Response, dataset Dataset) {
	answer, confident, err := routing.Choice(response, "route", dataset.MinConfidence)
	if response != nil {
		result.Model, result.Usage = response.Model, response.Usage
	}
	if err != nil {
		result.Error = err.Error()
		return
	}
	if _, ok := dataset.Question.Criteria[answer.Choice]; !ok {
		result.Error = "API selected an unconfigured choice"
		return
	}
	result.Choice, result.Confidence, result.Fallback = answer.Choice, answer.Confidence, !confident
	result.Correct = result.Fallback == result.ExpectedFallback &&
		(result.Fallback || result.Choice == result.ExpectedChoice)
}

func summarize(results []Result) *Report {
	report := &Report{Results: results, Total: len(results)}
	latencies := make([]float64, 0, len(results))
	models := make(map[string]bool)
	selectedCorrect := 0
	for _, result := range results {
		latencies = append(latencies, result.LatencyMS)
		report.Usage.InputTokens += result.Usage.InputTokens
		report.Usage.OutputTokens += result.Usage.OutputTokens
		if result.Model != "" {
			models[result.Model] = true
		}
		if result.Error != "" {
			report.Errors++
			continue
		}
		if result.Correct {
			report.Correct++
		}
		if result.Fallback {
			report.Fallbacks++
		} else {
			report.Selected++
			if result.Correct {
				selectedCorrect++
			}
		}
	}
	report.Models = slices.Sorted(maps.Keys(models))
	if report.Total > 0 {
		report.Accuracy = float64(report.Correct) / float64(report.Total)
		report.FallbackRate = float64(report.Fallbacks) / float64(report.Total)
		report.Coverage = float64(report.Selected) / float64(report.Total)
	}
	if report.Selected > 0 {
		report.SelectedAccuracy = float64(selectedCorrect) / float64(report.Selected)
	}
	slices.Sort(latencies)
	const median, tail = 0.5, 0.95
	report.P50LatencyMS, report.P95LatencyMS = percentile(latencies, median), percentile(latencies, tail)
	return report
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(math.Ceil(quantile*float64(len(sorted))))-1]
}
