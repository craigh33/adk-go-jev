package contextfilter

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/genai"
)

type group struct {
	start, end int
	pinned     bool
	text       string
}

type projection struct {
	text   string
	unsafe bool
}

func groupContents(ctx agent.Context, contents []*genai.Content, cfg Config) []group {
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
	var groups []group
	for i, content := range contents {
		boundary := i <= active && isUserMessage(content)
		if len(groups) == 0 || boundary {
			groups = append(groups, group{start: i, pinned: !boundary})
		}
		g := &groups[len(groups)-1]
		p := projectContent(content)
		g.end = i + 1
		g.text += p.text + "\n"
		g.pinned = g.pinned || i >= active || p.unsafe || containsSummary(content, summaries) ||
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

func projectContent(content *genai.Content) projection {
	if content == nil {
		return projection{unsafe: true}
	}
	p := projection{unsafe: content.Role != genai.RoleUser && content.Role != genai.RoleModel && content.Role != ""}
	var text strings.Builder
	text.WriteString(content.Role + ":\n")
	for _, part := range content.Parts {
		if part == nil {
			p.unsafe = true
			continue
		}
		if !part.Thought {
			text.WriteString(part.Text)
			text.WriteByte('\n')
		}
		var data any
		if call := part.FunctionCall; call != nil {
			data = map[string]any{"call": call.Name, "id": call.ID, "arguments": call.Args}
			p.unsafe = p.unsafe || len(call.PartialArgs) != 0 || call.WillContinue != nil
		}
		if response := part.FunctionResponse; response != nil {
			data = map[string]any{"result": response.Name, "id": response.ID, "response": response.Response}
			p.unsafe = p.unsafe || len(response.Parts) != 0 || response.WillContinue != nil || response.Scheduling != ""
		}
		p.unsafe = !writeToolText(&text, data) || p.unsafe
		remaining := *part
		remaining.Text, remaining.FunctionCall, remaining.FunctionResponse = "", nil, nil
		if !reflect.ValueOf(remaining).IsZero() {
			p.unsafe = true
			text.WriteString("[unreviewed content retained]\n")
		}
	}
	p.text = text.String()
	return p
}

func writeToolText(text *strings.Builder, data any) bool {
	if data == nil {
		return true
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		text.WriteString("[unreviewed tool data retained]\n")
		return false
	}
	text.Write(encoded)
	text.WriteByte('\n')
	return true
}

func protectToolPairs(contents []*genai.Content, groups []group) {
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

func protectPart(part *genai.Part, i int, groups []group, pending map[string]int) {
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
