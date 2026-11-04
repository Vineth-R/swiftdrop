// Package emails builds SwiftDrop's transactional emails: one branded layout
// (the web app's palette, logo and route motif, with a dark theme), a small set
// of content blocks, and one constructor per scenario (password reset,
// merchant approved, parcel delivered, and so on). Every email is rendered
// twice, as HTML and as plain text, from the same blocks, so the two cannot
// drift apart.
//
// A service builds an Email, turns it into a mailer.Message with Message and
// sends it with a mailer.Mailer. See docs/email.md.
package emails

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

//go:embed layout.html.tmpl
var layoutSource string

//go:embed assets/logo-mark.png
var logoMark []byte

const (
	logoCID     = "swiftdrop-logo"
	homeLabel   = "Open SwiftDrop"
	tagline     = "Courier and parcel network for Sri Lanka"
	fontDisplay = template.CSS(`'Bricolage Grotesque','Helvetica Neue',Helvetica,Arial,sans-serif`)
	fontBody    = template.CSS(`'Instrument Sans','Helvetica Neue',Helvetica,Arial,sans-serif`)
	fontMono    = template.CSS(`ui-monospace,'SFMono-Regular',Menlo,Consolas,'Liberation Mono',monospace`)

	// textWidth is the line length of the plain text version.
	textWidth = 72
)

var layout = template.Must(template.New("layout").Funcs(template.FuncMap{
	"lines": func(text string) []string { return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") },
	"inc":   func(i int) int { return i + 1 },
	"tone":  func(p palette, t Tone) toneColors { return p.tone(t) },
}).Parse(layoutSource))

// Site is where the emails point and who answers them.
type Site struct {
	// BaseURL is the web app's public address, such as https://app.swiftdrop.lk
	// (FRONTEND_BASE_URL). Every link in an email is built from it.
	BaseURL string
	// SupportEmail is shown in the footer. Empty leaves the line out.
	SupportEmail string
}

// URL returns an absolute link to a path of the web app, such as "/login".
func (s Site) URL(path string) string {
	return strings.TrimRight(s.BaseURL, "/") + path
}

// Email is one message, before it is rendered.
type Email struct {
	// Subject is the subject line. Keep it under 70 characters and say what the
	// email is about, not who sent it.
	Subject string
	// Preheader is the grey preview text a mail app shows beside the subject.
	Preheader string
	// Eyebrow is the small label above the heading, such as "Password reset".
	Eyebrow string
	// Heading is the one headline of the email.
	Heading string
	Blocks  []Block
	// Reason tells the reader why they got this email, in one sentence.
	Reason string
}

// Message renders the email for one recipient: a plain text part, an HTML part
// and the logo embedded in the message.
func (e Email) Message(to string, site Site) (mailer.Message, error) {
	if err := e.validate(site); err != nil {
		return mailer.Message{}, fmt.Errorf("email %q: %w", e.Subject, err)
	}

	data := struct {
		Email
		L, D                            palette
		LogoCID                         string
		Spacer                          template.HTML
		FontDisplay, FontBody, FontMono template.CSS
		SupportEmail, HomeURL           string
		HomeLabel                       string
	}{
		Email: e, L: light, D: dark, LogoCID: logoCID,
		// Pads the preview text so the mail app shows nothing from the body after it.
		Spacer:      template.HTML(strings.Repeat("&zwnj;&nbsp;", 60)),
		FontDisplay: fontDisplay, FontBody: fontBody, FontMono: fontMono,
		SupportEmail: site.SupportEmail, HomeURL: strings.TrimRight(site.BaseURL, "/"), HomeLabel: homeLabel,
	}
	var html bytes.Buffer
	if err := layout.Execute(&html, data); err != nil {
		return mailer.Message{}, fmt.Errorf("email %q: render html: %w", e.Subject, err)
	}

	return mailer.Message{
		To:      to,
		Subject: e.Subject,
		Text:    e.text(site),
		HTML:    html.String(),
		Inline:  []mailer.InlineImage{{ContentID: logoCID, ContentType: "image/png", Data: logoMark}},
	}, nil
}

func (e Email) validate(site Site) error {
	switch {
	case strings.TrimSpace(e.Subject) == "":
		return errors.New("subject is required")
	case strings.TrimSpace(e.Heading) == "":
		return errors.New("heading is required")
	case len(e.Blocks) == 0:
		return errors.New("an email needs at least one block")
	}
	if site.BaseURL != "" {
		if err := checkURL(site.BaseURL); err != nil {
			return fmt.Errorf("site: %w", err)
		}
	}
	buttons := 0
	for _, block := range e.Blocks {
		if err := block.validate(); err != nil {
			return err
		}
		if block.Kind == kindButton {
			buttons++
		}
	}
	if buttons > 1 {
		return errors.New("an email has one main action: use at most one button")
	}
	return nil
}

// text renders the plain text version: the same words as the HTML, wrapped for
// a mail app that shows only text.
func (e Email) text(site Site) string {
	var out strings.Builder
	line := func(text string) { out.WriteString(text + "\r\n") }
	blank := func() { out.WriteString("\r\n") }

	line("SWIFTDROP")
	blank()
	if e.Eyebrow != "" {
		line(strings.ToUpper(e.Eyebrow))
	}
	line(e.Heading)
	line(strings.Repeat("-", min(len([]rune(e.Heading)), textWidth)))
	blank()

	for _, block := range e.Blocks {
		switch block.Kind {
		case kindParagraph:
			line(wrap(block.Text, textWidth))
		case kindButton:
			line(block.Label + ":")
			line(block.URL)
		case kindDetails:
			for _, row := range block.Rows {
				line("  " + row.Label + ": " + row.Value)
			}
		case kindNotice:
			title := block.Title
			if title == "" {
				title = "Note"
			}
			line("[" + title + "]")
			line(wrap(block.Text, textWidth))
		case kindSteps:
			line(block.Title)
			for i, item := range block.Items {
				line(wrap(fmt.Sprintf("  %d. %s", i+1, item), textWidth))
			}
		}
		blank()
	}

	line("--")
	line("SwiftDrop - " + tagline)
	if e.Reason != "" {
		line(wrap(e.Reason, textWidth))
	}
	if site.SupportEmail != "" {
		line("Questions? Write to " + site.SupportEmail)
	}
	if site.BaseURL != "" {
		line(strings.TrimRight(site.BaseURL, "/"))
	}
	return out.String()
}

// wrap breaks text into lines of at most width characters at spaces, keeping
// existing line breaks and never splitting a word, so a link stays whole.
func wrap(text string, width int) string {
	var out []string
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		indent := paragraph[:len(paragraph)-len(strings.TrimLeft(paragraph, " "))]
		current := indent
		for _, word := range strings.Fields(paragraph) {
			switch {
			case strings.TrimSpace(current) == "":
				current = indent + word
			case len([]rune(current))+1+len([]rune(word)) > width:
				out = append(out, current)
				current = indent + word
			default:
				current += " " + word
			}
		}
		out = append(out, current)
	}
	return strings.Join(out, "\r\n")
}
