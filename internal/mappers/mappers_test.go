package mappers

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func TestResponseMap(t *testing.T) {
	t.Parallel()
	response := &typesafe.Response{
		Model: "jev-test", Answers: map[string]typesafe.Answer{"a": typesafe.NoulAnswer{Type: "noul", Noul: 0}},
		Usage: typesafe.Usage{InputTokens: 1, OutputTokens: 0},
	}
	result, err := ResponseMap(response)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got typesafe.Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	answer, ok := got.Answers["a"].(typesafe.NoulAnswer)
	if !ok || answer.Noul != 0 || got.Model != response.Model || got.Usage != response.Usage {
		t.Fatalf("response fields lost: %+v", got)
	}
	if _, err := ResponseMap(nil); err == nil {
		t.Fatal("accepted nil response")
	}
	response.Answers["a"] = typesafe.NoulAnswer{Noul: math.NaN()}
	if _, err := ResponseMap(response); err == nil {
		t.Fatal("accepted non-JSON response")
	}
}

func TestNoulProbability(t *testing.T) {
	t.Parallel()
	for _, probability := range []float64{0, 0.1, 1} {
		got, err := NoulProbability(typesafe.NoulAnswer{Noul: probability})
		if err != nil || got != probability {
			t.Fatalf("probability=%v: got=%v err=%v", probability, got, err)
		}
	}
	for _, answer := range []typesafe.Answer{
		nil, typesafe.ChoiceAnswer{}, typesafe.NoulAnswer{Noul: math.NaN()},
		typesafe.NoulAnswer{Noul: math.Inf(1)}, typesafe.NoulAnswer{Noul: -0.1}, typesafe.NoulAnswer{Noul: 1.1},
	} {
		if _, err := NoulProbability(answer); err == nil {
			t.Fatalf("accepted invalid probability: %v", answer)
		}
	}
}
