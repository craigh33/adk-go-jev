package contextfilter

import "github.com/craigh33/adk-go-typesafe/typesafe"

// Decision describes an assessed turn in the original request's Contents.
type Decision struct {
	Start     int // Inclusive message index.
	End       int // Exclusive message index.
	Relevance float64
	Remove    bool // Proposed removal, including in Observe mode.
}

// Report contains assessments, removals and Jev usage. Failed evaluations may
// incur unreported usage. Unassessed turns are retained.
type Report struct {
	Decisions    []Decision
	RemovedTurns int
	Usage        typesafe.Usage
	Err          error
}

type turn struct {
	start, end int
	pinned     bool
	text       string
}

// reviewState contains the evidence for the turn assessments.
type reviewState struct {
	LatestRequest string        `json:"latest_request"`
	Instructions  string        `json:"instructions,omitempty"`
	Groups        []reviewGroup `json:"groups"`
}

// reviewGroup identifies a selected turn and its projected text.
type reviewGroup struct {
	ID     string `json:"id"`
	Pinned bool   `json:"pinned"`
	Text   string `json:"text"`
}
