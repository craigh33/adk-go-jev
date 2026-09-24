package contextfilter

import (
	"reflect"
	"slices"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/genai"

	"github.com/craigh33/adk-go-typesafe/internal/mappers"
)

type turn struct {
	start, end int
	pinned     bool
	text       string
}

func groupTurns(ctx agent.Context, contents []*genai.Content, cfg Config) []turn {
	active := -1
	for i, content := range contents {
		if content != nil && reflect.DeepEqual(content, ctx.UserContent()) {
			active = i
		}
	}
	// A transformed or delegated input cannot safely establish the active turn.
	if active < 0 {
		return nil
	}
	summaries := summaryTexts(ctx)
	var groups []turn
	for i, content := range contents {
		boundary := i <= active && isUserMessage(content)
		if len(groups) == 0 || boundary {
			groups = append(groups, turn{start: i, pinned: !boundary})
		}
		g := &groups[len(groups)-1]
		p := mappers.ContextContent(content)
		g.end = i + 1
		g.text += p.Text + "\n"
		g.pinned = g.pinned || i >= active || p.Unsupported || containsSummary(content, summaries) ||
			(cfg.Pin != nil && cfg.Pin(ctx, content))
	}
	for i := max(0, len(groups)-cfg.KeepRecentTurns); i < len(groups); i++ {
		groups[i].pinned = true
	}
	protectToolPairs(contents, groups)
	return groups
}

func isUserMessage(content *genai.Content) bool {
	if content == nil || content.Role != genai.RoleUser {
		return false
	}
	return !slices.ContainsFunc(content.Parts, func(part *genai.Part) bool {
		return part != nil && (part.FunctionCall != nil || part.FunctionResponse != nil)
	})
}

func summaryTexts(ctx agent.Context) map[string]bool {
	texts := make(map[string]bool)
	if ctx.Session() != nil {
		for event := range ctx.Session().Events().All() {
			if event == nil || event.Actions.Compaction == nil || event.Actions.Compaction.CompactedContent == nil {
				continue
			}
			for _, part := range event.Actions.Compaction.CompactedContent.Parts {
				if part != nil && part.Text != "" {
					texts[part.Text] = true
				}
			}
		}
	}
	return texts
}

func containsSummary(content *genai.Content, summaries map[string]bool) bool {
	return content != nil && slices.ContainsFunc(content.Parts, func(part *genai.Part) bool {
		return part != nil && summaries[part.Text]
	})
}

func protectToolPairs(contents []*genai.Content, groups []turn) {
	pending := make(map[string]int)
	for i := range groups {
		for _, content := range contents[groups[i].start:groups[i].end] {
			if content == nil {
				continue
			}
			for _, part := range content.Parts {
				protectPart(part, i, groups, pending)
			}
		}
	}
	for _, i := range pending {
		groups[i].pinned = true
	}
}

func protectPart(part *genai.Part, i int, groups []turn, pending map[string]int) {
	if part == nil {
		return
	}
	if call := part.FunctionCall; call != nil {
		key := toolKey(call.ID, call.Name)
		if other, found := pending[key]; found {
			groups[other].pinned, groups[i].pinned = true, true
		}
		pending[key] = i
	}
	if response := part.FunctionResponse; response != nil {
		key := toolKey(response.ID, response.Name)
		other, found := pending[key]
		if !found || other != i {
			groups[i].pinned = true
			if found {
				groups[other].pinned = true
			}
		}
		delete(pending, key)
	}
}

func toolKey(id, name string) string {
	if id != "" {
		return "id:" + id
	}
	return "name:" + name
}

func selectTurns(contents []*genai.Content, decisions []Decision) ([]*genai.Content, int) {
	var selected []*genai.Content
	removed, start := 0, 0
	for _, decision := range decisions {
		if decision.Remove {
			selected = append(selected, contents[start:decision.Start]...)
			start = decision.End
			removed++
		}
	}
	if removed == 0 {
		return contents, 0
	}
	return append(selected, contents[start:]...), removed
}
