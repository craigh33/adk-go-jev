//go:build integration

package typesafe_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestLiveSystemOne(t *testing.T) {
	client, err := typesafe.New(&typesafe.Options{Model: os.Getenv("TYPESAFE_MODEL")})
	if err != nil {
		t.Fatal("live tests require TYPESAFE_API_KEY: ", err)
	}
	states := map[string]any{
		"text": "I was charged twice. Please refund the duplicate payment.",
		"structured": map[string]any{
			"message": "The integration times out",
			"history": []any{"Please help", map[string]any{"attempts": 2}},
		},
	}
	for name, state := range states {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			started := time.Now()
			response, err := client.Evaluate(
				ctx,
				&typesafe.Request{State: state, Questions: map[string]typesafe.Question{
					"team": typesafe.Choice{
						Instructions: "Which team should handle this?",
						Criteria: map[string]any{
							"billing":   "Payments and refunds",
							"technical": "Bugs and integrations",
						},
					},
					"urgency": typesafe.Score{
						Instructions: "How urgent is this?",
						Criteria:     []any{"Can wait", "Soon", "Immediately"},
					},
					"refund": typesafe.Noul{Instructions: "Does the customer request a refund?"},
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.Answers["team"].(typesafe.ChoiceAnswer); !ok {
				t.Fatal("missing choice answer")
			}
			if _, ok := response.Answers["urgency"].(typesafe.ScoreAnswer); !ok {
				t.Fatal("missing score answer")
			}
			if _, ok := response.Answers["refund"].(typesafe.NoulAnswer); !ok {
				t.Fatal("missing noul answer")
			}
			if response.Model == "" || response.Usage.InputTokens <= 0 {
				t.Fatal("missing model or input usage")
			}
			t.Logf(
				"model=%s elapsed=%s input_tokens=%d output_tokens=%d",
				response.Model,
				time.Since(started),
				response.Usage.InputTokens,
				response.Usage.OutputTokens,
			)
		})
	}
}
