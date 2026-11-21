package emails

import (
	"fmt"
	"net/url"
	"strings"
)

// Tone colours a notice, like the web app's Alert.
type Tone string

const (
	ToneInfo    Tone = "info"
	ToneSuccess Tone = "success"
	ToneWarning Tone = "warning"
	ToneDanger  Tone = "danger"
)

type blockKind string

const (
	kindParagraph blockKind = "paragraph"
	kindButton    blockKind = "button"
	kindDetails   blockKind = "details"
	kindNotice    blockKind = "notice"
	kindSteps     blockKind = "steps"
)

// Block is one piece of an email's body. Build them with Paragraph, Button,
// Details, Notice and Steps; the layout gives every email the same look.
type Block struct {
	Kind  blockKind
	Text  string
	Label string
	URL   string
	Rows  []Row
	Tone  Tone
	Title string
	Items []string
}

// Row is one label and value in a Details block.
type Row struct {
	Label string
	Value string
	// IsMono sets identifiers (tracking numbers, key prefixes) in a monospaced
	// font, as the web app does.
	IsMono bool
}

// Paragraph is a paragraph of text. A line break ("\n") starts a new line.
func Paragraph(text string) Block { return Block{Kind: kindParagraph, Text: text} }

// Button is the one action of the email, with its link repeated as text under
// it for clients that block buttons. Use at most one per email. The URL must be
// absolute (http or https).
func Button(label, url string) Block { return Block{Kind: kindButton, Label: label, URL: url} }

// Details is a short table of facts: the order, the business, the time.
func Details(rows ...Row) Block { return Block{Kind: kindDetails, Rows: rows} }

// Notice is a boxed message that stands out from the paragraphs, such as "This
// link works once" or the reason a merchant was rejected.
func Notice(tone Tone, title, text string) Block {
	return Block{Kind: kindNotice, Tone: tone, Title: title, Text: text}
}

// Steps is a short numbered list with a title, such as "What happens next".
func Steps(title string, items ...string) Block {
	return Block{Kind: kindSteps, Title: title, Items: items}
}

// validate checks what a template cannot: that a link is absolute and safe.
func (b Block) validate() error {
	switch b.Kind {
	case kindParagraph:
		if strings.TrimSpace(b.Text) == "" {
			return fmt.Errorf("paragraph is empty")
		}
	case kindButton:
		if strings.TrimSpace(b.Label) == "" {
			return fmt.Errorf("button has no label")
		}
		return checkURL(b.URL)
	case kindDetails:
		if len(b.Rows) == 0 {
			return fmt.Errorf("details has no rows")
		}
	case kindNotice:
		if strings.TrimSpace(b.Text) == "" {
			return fmt.Errorf("notice has no text")
		}
	case kindSteps:
		if len(b.Items) == 0 {
			return fmt.Errorf("steps has no items")
		}
	default:
		return fmt.Errorf("unknown block %q", b.Kind)
	}
	return nil
}

// checkURL accepts only absolute http and https links, so an email never
// carries a javascript: link or a link that depends on where it is read.
func checkURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("link %q must be an absolute http or https URL", raw)
	}
	return nil
}
