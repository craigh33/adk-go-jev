package mappers

import (
	"fmt"
	"math"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

// ContextRelevance validates and extracts a turn's Noul probability.
func ContextRelevance(answer typesafe.Answer, turnID string) (float64, error) {
	value, ok := answer.(typesafe.NoulAnswer)
	if !ok || math.IsNaN(value.Noul) || value.Noul < 0 || value.Noul > 1 {
		return 0, fmt.Errorf("contextfilter: invalid relevance for turn at %s", turnID)
	}
	return value.Noul, nil
}
