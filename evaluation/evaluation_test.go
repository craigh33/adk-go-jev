package evaluation

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

type fakeAPI func(context.Context, *typesafe.Request) (*typesafe.Response, error)

func (f fakeAPI) Evaluate(ctx context.Context, req *typesafe.Request) (*typesafe.Response, error) {
	return f(ctx, req)
}

func dataset() Dataset {
	return Dataset{
		Model:         "jev-pinned",
		Question:      typesafe.Choice{Criteria: map[string]any{"billing": "Payments", "technical": "Bugs"}},
		MinConfidence: 0.75,
		Cases: []Case{
			{Name: "correct", State: "one", ExpectedChoice: "billing"},
			{Name: "wrong", State: map[string]any{"text": "two"}, ExpectedChoice: "technical"},
			{Name: "fallback", State: "three", ExpectedFallback: true},
			{Name: "failure", State: "four", ExpectedChoice: "billing"},
		},
	}
}

func TestRunMetrics(t *testing.T) {
	t.Parallel()
	d := dataset()
	calls := 0
	api := fakeAPI(func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) {
		if req.Model != d.Model || !reflect.DeepEqual(req.State, d.Cases[calls].State) {
			t.Errorf("request = %#v", req)
		}
		calls++
		if calls == 4 {
			return nil, errors.New("service unavailable")
		}
		confidence := 0.75
		if calls == 3 {
			confidence = 0.5
		}
		return &typesafe.Response{
			Model: "jev-resolved",
			Usage: typesafe.Usage{InputTokens: 10, OutputTokens: 2},
			Answers: map[string]typesafe.Answer{
				"route": typesafe.ChoiceAnswer{Choice: "billing", Confidence: confidence},
			},
		}, nil
	})
	report, err := Run(t.Context(), api, d)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 4 || report.Correct != 2 || report.Selected != 2 || report.Fallbacks != 1 || report.Errors != 1 {
		t.Fatalf("counts = %+v", report)
	}
	if report.Accuracy != 0.5 || report.SelectedAccuracy != 0.5 || report.FallbackRate != 0.25 ||
		report.Coverage != 0.5 {
		t.Fatalf("metrics = %+v", report)
	}
	if report.Usage.InputTokens != 30 || report.Usage.OutputTokens != 6 ||
		!reflect.DeepEqual(report.Models, []string{"jev-resolved"}) {
		t.Fatalf("metadata = %+v", report)
	}
	if report.P50LatencyMS < 0 || report.P95LatencyMS < report.P50LatencyMS {
		t.Fatal("invalid latencies")
	}
}

func TestRunCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	api := fakeAPI(func(context.Context, *typesafe.Request) (*typesafe.Response, error) {
		calls++
		cancel()
		return nil, context.Canceled
	})
	report, err := Run(ctx, api, dataset())
	if !errors.Is(err, context.Canceled) || calls != 1 || report.Total != 1 || report.Errors != 1 {
		t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
	}
	report, err = Run(ctx, api, dataset())
	if !errors.Is(err, context.Canceled) || calls != 1 || report.Total != 0 || report.Accuracy != 0 {
		t.Fatalf("pre-canceled report=%+v err=%v", report, err)
	}
}

func TestInvalidDatasetBeforeCalls(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*Dataset){
		"empty":             func(d *Dataset) { d.Cases = nil },
		"duplicate":         func(d *Dataset) { d.Cases[1].Name = d.Cases[0].Name },
		"blank name":        func(d *Dataset) { d.Cases[0].Name = " " },
		"unknown choice":    func(d *Dataset) { d.Cases[0].ExpectedChoice = "sales" },
		"both expectations": func(d *Dataset) { d.Cases[0].ExpectedFallback = true },
		"invalid state":     func(d *Dataset) { d.Cases[3].State = nil },
		"invalid threshold": func(d *Dataset) { d.MinConfidence = math.NaN() },
		"invalid rubric":    func(d *Dataset) { d.Question.Criteria = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := dataset()
			edit(&d)
			api := fakeAPI(func(context.Context, *typesafe.Request) (*typesafe.Response, error) {
				t.Error("invalid dataset made API call")
				return nil, errors.New("unexpected API call")
			})
			if _, err := Run(t.Context(), api, d); err == nil {
				t.Fatal("accepted invalid dataset")
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()
	const valid = `{"question":{"criteria":{"billing":"Payments"}},"min_confidence":0.7,"cases":[{"name":"one","state":"text","expected_choice":"billing"}]}`
	if _, err := Load(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{valid + ` {}`, valid + ` garbage`, strings.Replace(valid, `"state"`, `"typo"`, 1), strings.Replace(valid, `"criteria"`, `"criterion"`, 1)} {
		if _, err := Load(strings.NewReader(value)); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}

func TestInvalidAnswers(t *testing.T) {
	t.Parallel()
	for _, response := range []*typesafe.Response{
		nil,
		{Answers: map[string]typesafe.Answer{"route": typesafe.NoulAnswer{Noul: 1}}},
		{Answers: map[string]typesafe.Answer{"route": typesafe.ChoiceAnswer{Choice: "billing", Confidence: math.Inf(1)}}},
		{Answers: map[string]typesafe.Answer{"route": typesafe.ChoiceAnswer{Choice: "unknown", Confidence: 1}}},
	} {
		d := dataset()
		d.Cases = d.Cases[:1]
		report, err := Run(
			t.Context(),
			fakeAPI(func(context.Context, *typesafe.Request) (*typesafe.Response, error) { return response, nil }),
			d,
		)
		if err != nil || report.Errors != 1 || report.Selected != 0 || report.Fallbacks != 0 {
			t.Fatalf("report=%+v err=%v", report, err)
		}
	}
}

func TestLatencyPercentiles(t *testing.T) {
	t.Parallel()
	report := summarize([]Result{{LatencyMS: 40}, {LatencyMS: 10}, {LatencyMS: 20}, {LatencyMS: 30}})
	if report.P50LatencyMS != 20 || report.P95LatencyMS != 40 {
		t.Fatalf("percentiles = %v, %v", report.P50LatencyMS, report.P95LatencyMS)
	}
}
