package parcels

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
)

func TestReceiptUnicodeShapingAndSearchLayer(t *testing.T) {
	cases := []struct {
		name, value string
		rtl         bool
	}{{"latin", "José François Müller", false}, {"latin_decomposed", "Jose\u0301 Franc\u0327ois", false}, {"sinhala", "නිමල් පෙරේරා", false}, {"tamil", "அமலி பெர்னாண்டோ", false}, {"arabic", "محمد أحمد", true}, {"arabic_marks", "مُحَمَّد أَحْمَد", true}, {"hebrew", "דוד לוי", true}, {"hebrew_marks", "דָּוִד לֵוִי", true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := newReceiptText()
			if err != nil {
				t.Fatal(err)
			}
			if receiptHasUnsupportedRune(text, tc.value) {
				t.Fatal("bundled fonts lack a requested glyph")
			}
			rows := text.layout("Sender: " + tc.value)
			if len(rows) != 1 {
				t.Fatalf("unexpected rows%d", len(rows))
			}
			var paths strings.Builder
			text.draw(&paths, rows[0], 48, 790)
			if !strings.Contains(paths.String(), " cm\n") || !strings.Contains(paths.String(), "f Q") {
				t.Fatal("Unicode text has no visible glyph paths")
			}
			hasRTL := false
			for _, run := range rows[0].runs {
				if run.Direction.Progression() == di.TowardTopLeft {
					hasRTL = true
				}
				for _, glyph := range run.Glyphs {
					if glyph.GlyphID == 0 || glyph.GlyphID == font.EmptyGlyph {
						t.Fatalf("missing glyph for script %s", tc.name)
					}
				}
			}
			if hasRTL != tc.rtl {
				t.Fatalf("wrong script direction: RTL%v", hasRTL)
			}
			p := Detail{TrackingNumber: "SDKABCDEFGH", Status: "CREATED", Sender: Contact{Name: tc.value, City: "Colombo"}, Recipient: Contact{Name: "Recipient", City: "Kandy"}, WeightKG: 200, SizeCategory: "SMALL", Fee: Fee{"LKR", 45000, 10000, 0, 55000}, CreatedAt: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
			pdf, err := Receipt(p)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"/ToUnicode", "/FontFile2", "3 Tr", "f Q"} {
				if !strings.Contains(string(pdf), required) {
					t.Fatalf("missing real Unicode PDF component %s", required)
				}
			}
			logical := newReceiptUnicode(rows)
			raw, err := hex.DecodeString(logical.encode(rows[0].text))
			if err != nil || len(raw) != 2*len([]rune(rows[0].text)) {
				t.Fatal("logical Unicode text was truncated")
			}
			if dir := os.Getenv("M5_RECEIPT_ARTIFACT_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".pdf"), pdf, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestReceiptUnicodeWrapPreservesClusters(t *testing.T) {
	text, err := newReceiptText()
	if err != nil {
		t.Fatal(err)
	}
	value := strings.Repeat("ක්‍රියාකාරී ", 35)
	rows := text.layout(value)
	if len(rows) < 2 {
		t.Fatal("long Unicode name did not wrap")
	}
	var combined strings.Builder
	for _, row := range rows {
		combined.WriteString(row.text)
		width := 0.0
		for _, run := range row.runs {
			width += float64(run.Advance) / 64
		}
		if width > 450.1 {
			t.Fatalf("wrapped line exceeds page: %f", width)
		}
	}
	if combined.String() != value {
		t.Fatalf("wrapping lost Unicode text\n%s\n%s", value, combined.String())
	}
	if receiptHasUnsupportedRune(text, "Jane") || !receiptHasUnsupportedRune(text, "漢字") {
		t.Fatal("font coverage boundaries are not detected")
	}
}

// Keep the shaping output tied to the library's cluster-aware wrapper API.
var _ shaping.Fontmap = (*receiptText)(nil)
