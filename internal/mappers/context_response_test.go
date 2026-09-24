package mappers

import (
	"math"
	"testing"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestContextRelevance(t *testing.T) {
	t.Parallel()
	for _, probability := range []float64{0, 0.1, 1} {
		got, err := ContextRelevance(typesafe.NoulAnswer{Noul: probability}, "0")
		if err != nil || got != probability {
			t.Fatalf("probability=%v: got=%v err=%v", probability, got, err)
		}
	}
	for _, answer := range []typesafe.Answer{
		nil, typesafe.ChoiceAnswer{}, typesafe.NoulAnswer{Noul: math.NaN()},
		typesafe.NoulAnswer{Noul: math.Inf(1)}, typesafe.NoulAnswer{Noul: -0.1}, typesafe.NoulAnswer{Noul: 1.1},
	} {
		if _, err := ContextRelevance(answer, "0"); err == nil {
			t.Fatalf("accepted invalid relevance: %v", answer)
		}
	}
}
