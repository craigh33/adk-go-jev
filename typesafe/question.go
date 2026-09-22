package typesafe

import (
	"encoding/json"

	apitypes "github.com/craigh33/adk-go-typesafe/internal/typesafe"
)

const (
	choiceType = "choice"
	scoreType  = "score"
	noulType   = "noul"
)

// Question is a Choice, Score, or Noul question. Its data fields come from the
// generated API contract; the wrappers supply the correct type discriminator.
type Question interface {
	json.Marshaler
	kind() string
}

// Choice selects one of up to 255 named options. Instructions and descriptions
// accept strings, objects, arrays, or nil. Type is set automatically when encoded.
type Choice apitypes.ChoiceQuestion

func (q Choice) kind() string { return choiceType }

// MarshalJSON sets the API's question discriminator.
func (q Choice) MarshalJSON() ([]byte, error) {
	wire := apitypes.ChoiceQuestion(q)
	wire.Type = q.kind()
	return json.Marshal(wire)
}

// Score evaluates ordered levels, from zero to len(Criteria)-1. Instructions may
// be nil; level descriptions must be strings, objects, or arrays.
// Type is set automatically when encoded.
type Score apitypes.ScoreQuestion

func (q Score) kind() string { return scoreType }

// MarshalJSON sets the API's question discriminator.
func (q Score) MarshalJSON() ([]byte, error) {
	wire := apitypes.ScoreQuestion(q)
	wire.Type = q.kind()
	return json.Marshal(wire)
}

// Noul evaluates a yes/no question and returns the probability of yes.
// Type is set automatically when encoded.
type Noul apitypes.NoulQuestion

func (q Noul) kind() string { return noulType }

// MarshalJSON sets the API's question discriminator.
func (q Noul) MarshalJSON() ([]byte, error) {
	wire := apitypes.NoulQuestion(q)
	wire.Type = q.kind()
	return json.Marshal(wire)
}

// NoulCriteria describes what yes and no mean for a Noul question.
type NoulCriteria = apitypes.NoulCriteria
