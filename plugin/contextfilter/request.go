package contextfilter

import (
	"fmt"
	"strconv"

	"google.golang.org/adk/v2/model"

	"github.com/craigh33/adk-go-typesafe/internal/adkcontent"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func (p *contextFilter) newRequest(request *model.LLMRequest, turns []turn, latest string) *typesafe.Request {
	state := reviewState{LatestRequest: latest}
	if request.Config != nil {
		state.Instructions = adkcontent.Text(request.Config.SystemInstruction).Text
	}
	questions := make(map[string]typesafe.Question)
	for _, turn := range turns {
		id := strconv.Itoa(turn.start)
		state.Groups = append(state.Groups, reviewGroup{ID: id, Pinned: turn.pinned, Text: turn.text})
		if turn.pinned {
			continue
		}
		questions[id] = typesafe.Noul{
			Instructions: fmt.Sprintf(relevanceInstructions, id),
			Criteria: &typesafe.NoulCriteria{
				True: relevantCriteria, False: irrelevantCriteria,
			},
		}
	}
	return &typesafe.Request{Model: p.cfg.Model, State: state, Questions: questions}
}
