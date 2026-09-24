package contextfilter

import "github.com/craigh33/adk-go-typesafe/typesafe"

// Decision describes an assessed turn in the original request's Contents.
type Decision struct {
	Start     int // Inclusive message index.
	End       int // Exclusive message index.
	Relevance float64
	Remove    bool // Proposed removal, including in Observe mode.
}

// Report contains successful assessments and actual removals. Usage sums on-time
// Jev responses; failures may incur unreported usage. Unassessed turns are retained.
type Report struct {
	Decisions    []Decision
	RemovedTurns int
	Usage        typesafe.Usage
	Err          error
}
