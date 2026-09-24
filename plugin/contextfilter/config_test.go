package contextfilter

import (
	"context"
	"math"
	"testing"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	api := evaluatorFunc(
		func(_ context.Context, req *typesafe.Request) (*typesafe.Response, error) { return scores(req, 0), nil },
	)
	for _, cfg := range []Config{
		{}, {API: api, RemovalThreshold: -1}, {API: api, RemovalThreshold: 2},
		{API: api, RemovalThreshold: math.NaN()}, {API: api, RemovalThreshold: math.Inf(1)},
		{API: api, KeepRecentTurns: -1}, {API: api, MinBytes: -1}, {API: api, MaxBatchBytes: -1}, {API: api, Timeout: -1},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("accepted invalid config: %+v", cfg)
		}
	}
	filter, err := New(Config{API: api})
	if err != nil {
		t.Fatal(err)
	}
	if filter.Name() != "context_filter" {
		t.Fatalf("unexpected plugin name: %q", filter.Name())
	}
	if _, err := filter.BeforeModelCallback()(callbackContext{parent: t.Context()}, nil); err == nil {
		t.Fatal("accepted nil request")
	}
	custom, err := New(Config{API: api, Name: "history"})
	if err != nil || custom.Name() != "history" {
		t.Fatalf("custom plugin name was not used: %v", err)
	}
}
