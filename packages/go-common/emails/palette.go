package emails

import "html/template"

// The email palette is the web app's tokens (apps/web/src/app/globals.css),
// written out because an email cannot read a stylesheet from the app. Keep the
// values in step with the tokens: a colour that changes there changes here.

type toneColors struct {
	Fg     template.CSS // text and the strong left edge
	Tint   template.CSS // the background
	Border template.CSS
}

type palette struct {
	Paper, Surface, Surface2, Line, LineStrong template.CSS
	Text, Muted, Faint                         template.CSS
	Primary, OnPrimary                         template.CSS // the button
	Accent                                     template.CSS // lime, the brand node
	Link                                       template.CSS // teal
	Info, Success, Warning, Danger             toneColors
}

var light = palette{
	Paper: "#f6f4ef", Surface: "#ffffff", Surface2: "#efede6", Line: "#dad6cb", LineStrong: "#bdb8aa",
	Text: "#14171a", Muted: "#5c6068", Faint: "#686c74",
	Primary: "#14171a", OnPrimary: "#f6f4ef",
	Accent:  "#d6f04a",
	Link:    "#0f6b6b",
	Info:    toneColors{Fg: "#2a5fa8", Tint: "#e8eff9", Border: "#bdd0ec"},
	Success: toneColors{Fg: "#1e7a4c", Tint: "#e4f1e9", Border: "#b5d8c4"},
	Warning: toneColors{Fg: "#9a5b00", Tint: "#fbf0dc", Border: "#ead3a6"},
	Danger:  toneColors{Fg: "#c23b2e", Tint: "#fbe9e6", Border: "#efc4bd"},
}

var dark = palette{
	Paper: "#0e1113", Surface: "#171b1e", Surface2: "#1f2427", Line: "#2b3135", LineStrong: "#3c4347",
	Text: "#eceae4", Muted: "#a8adb4", Faint: "#a8adb4",
	Primary: "#eceae4", OnPrimary: "#14171a",
	Accent:  "#d6f04a",
	Link:    "#4fc8c0",
	Info:    toneColors{Fg: "#6e9fe0", Tint: "#131d2b", Border: "#22334a"},
	Success: toneColors{Fg: "#4fb87c", Tint: "#122018", Border: "#1f3a2a"},
	Warning: toneColors{Fg: "#e09a3c", Tint: "#241b0c", Border: "#40301a"},
	Danger:  toneColors{Fg: "#f2705f", Tint: "#2a1512", Border: "#4a2620"},
}

// tone returns the colours of one tone in a palette.
func (p palette) tone(t Tone) toneColors {
	switch t {
	case ToneSuccess:
		return p.Success
	case ToneWarning:
		return p.Warning
	case ToneDanger:
		return p.Danger
	default:
		return p.Info
	}
}
