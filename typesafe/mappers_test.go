package typesafe

import (
	"encoding/json"
	"math"
	"testing"
)

func TestResponseMap(t *testing.T) {
	t.Parallel()
	response := &Response{
		Model: "jev-test", Answers: map[string]Answer{"a": NoulAnswer{Type: "noul", Noul: 0}},
		Usage: Usage{InputTokens: 1, OutputTokens: 0},
	}
	result, err := ResponseMap(response)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	answer, ok := got.Answers["a"].(NoulAnswer)
	if !ok || answer.Noul != 0 || got.Model != response.Model || got.Usage != response.Usage {
		t.Fatalf("response fields lost: %+v", got)
	}
	if _, err := ResponseMap(nil); err == nil {
		t.Fatal("accepted nil response")
	}
	response.Answers["a"] = NoulAnswer{Noul: math.NaN()}
	if _, err := ResponseMap(response); err == nil {
		t.Fatal("accepted non-JSON response")
	}
}

func TestNoulProbability(t *testing.T) {
	t.Parallel()
	for _, probability := range []float64{0, 0.1, 1} {
		got, err := NoulProbability(NoulAnswer{Noul: probability})
		if err != nil || got != probability {
			t.Fatalf("probability=%v: got=%v err=%v", probability, got, err)
		}
	}
	for _, answer := range []Answer{
		nil, ChoiceAnswer{}, NoulAnswer{Noul: math.NaN()},
		NoulAnswer{Noul: math.Inf(1)}, NoulAnswer{Noul: -0.1}, NoulAnswer{Noul: 1.1},
	} {
		if _, err := NoulProbability(answer); err == nil {
			t.Fatalf("accepted invalid probability: %v", answer)
		}
	}
}
