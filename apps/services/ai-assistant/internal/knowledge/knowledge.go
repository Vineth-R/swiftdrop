// Package knowledge loads the version-controlled SwiftDrop knowledge base and
// retrieves the most relevant sections with lightweight keyword scoring. It
// deliberately uses no embeddings, vector database or external service.
package knowledge

import (
	"embed"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

//go:embed data/*.md
var files embed.FS

// Section is one retrievable Markdown section.
type Section struct {
	Source  string
	Heading string
	Content string
}

// Base is the loaded, immutable knowledge base.
type Base struct {
	sections []Section
	document map[string]int
}

// Load parses the embedded Markdown knowledge base.
func Load() (*Base, error) {
	entries, err := files.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("read embedded knowledge: %w", err)
	}

	base := &Base{document: make(map[string]int)}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := files.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read knowledge file %s: %w", entry.Name(), err)
		}
		sections := parse(entry.Name(), string(raw))
		base.sections = append(base.sections, sections...)
	}

	if len(base.sections) == 0 {
		return nil, fmt.Errorf("knowledge base is empty")
	}

	for _, section := range base.sections {
		seen := make(map[string]struct{})
		for _, token := range tokenize(section.Source + " " + section.Heading + " " + section.Content) {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			base.document[token]++
		}
	}
	return base, nil
}

// Sections exposes a copy of every section for tests and diagnostics.
func (b *Base) Sections() []Section {
	if b == nil {
		return nil
	}
	return append([]Section(nil), b.sections...)
}

// Retrieve returns up to limit relevant sections ordered by descending score.
// It returns nil when the query has no meaningful overlap with approved
// knowledge; callers should then rely on the system prompt and the model's
// clarification behaviour rather than inventing content.
func (b *Base) Retrieve(query string, limit int) []Section {
	if b == nil || limit < 1 {
		return nil
	}

	tokens := uniqueTokens(tokenize(query))
	if len(tokens) == 0 {
		return nil
	}

	type scored struct {
		section Section
		score   float64
	}
	scoredSections := make([]scored, 0, len(b.sections))
	total := float64(len(b.sections))

	for _, section := range b.sections {
		heading := strings.ToLower(section.Heading)
		source := strings.ToLower(section.Source)
		content := strings.ToLower(section.Content)
		score := 0.0
		for _, token := range tokens {
			frequency := strings.Count(content, token)
			if frequency == 0 && !strings.Contains(heading, token) && !strings.Contains(source, token) {
				continue
			}
			weight := math.Log(1 + total/(1+float64(b.document[token])))
			if weight <= 0 {
				weight = 1
			}
			score += float64(frequency) * weight
			if strings.Contains(heading, token) {
				score += 3 * weight
			}
		}
		if score > 0 {
			scoredSections = append(scoredSections, scored{section: section, score: score})
		}
	}

	sort.SliceStable(scoredSections, func(i, j int) bool {
		if scoredSections[i].score == scoredSections[j].score {
			if scoredSections[i].section.Source == scoredSections[j].section.Source {
				return scoredSections[i].section.Heading < scoredSections[j].section.Heading
			}
			return scoredSections[i].section.Source < scoredSections[j].section.Source
		}
		return scoredSections[i].score > scoredSections[j].score
	})

	if limit > len(scoredSections) {
		limit = len(scoredSections)
	}
	result := make([]Section, 0, limit)
	for _, item := range scoredSections[:limit] {
		result = append(result, item.section)
	}
	return result
}

// Format renders retrieved sections for the system instruction.
func Format(sections []Section) string {
	var builder strings.Builder
	for _, section := range sections {
		builder.WriteString("### ")
		builder.WriteString(section.Source)
		builder.WriteString(" — ")
		builder.WriteString(section.Heading)
		builder.WriteByte('\n')
		builder.WriteString(strings.TrimSpace(section.Content))
		builder.WriteString("\n\n")
	}
	return strings.TrimSpace(builder.String())
}

func parse(source, raw string) []Section {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	documentTitle := strings.TrimSuffix(source, ".md")
	sections := make([]Section, 0, 4)

	heading := ""
	var body []string
	flush := func() {
		content := strings.TrimSpace(strings.Join(body, "\n"))
		if content == "" {
			body = nil
			return
		}
		title := documentTitle
		if heading != "" {
			title = documentTitle + " > " + heading
		}
		sections = append(sections, Section{Source: source, Heading: title, Content: content})
		body = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			documentTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		case strings.HasPrefix(trimmed, "## "):
			flush()
			heading = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		default:
			body = append(body, line)
		}
	}
	flush()
	return sections
}

var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "are": {}, "but": {}, "not": {}, "you": {},
	"your": {}, "with": {}, "this": {}, "that": {}, "from": {}, "have": {},
	"has": {}, "was": {}, "were": {}, "will": {}, "can": {}, "how": {}, "what": {},
	"when": {}, "where": {}, "which": {}, "who": {}, "why": {}, "does": {}, "did": {},
	"about": {}, "into": {}, "than": {}, "then": {}, "there": {}, "their": {},
	"them": {}, "they": {}, "also": {}, "use": {}, "using": {}, "get": {}, "got": {},
	"please": {}, "would": {}, "could": {}, "should": {}, "may": {}, "might": {},
	"explain": {}, "tell": {}, "show": {}, "give": {}, "describe": {}, "define": {},
	"meaning": {}, "help": {}, "need": {}, "want": {}, "work": {}, "works": {},
	"working": {}, "happen": {}, "happens": {},
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if len([]rune(field)) < 2 {
			continue
		}
		if _, stop := stopwords[field]; stop {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func uniqueTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	return result
}
