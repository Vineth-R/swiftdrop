package parcels

import (
	"bytes"
	"compress/zlib"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/bidi"
)

//go:embed receiptfonts/*.ttf receiptfonts/OFL.txt
var receiptFontAssets embed.FS

var receiptFontFiles = []string{"NotoSans-Regular.ttf", "NotoSansSinhala-Regular.ttf", "NotoSansTamil-Regular.ttf", "NotoSansArabic-Regular.ttf", "NotoSansHebrew-Regular.ttf"}

type receiptText struct {
	faces  []*font.Face
	shaper shaping.HarfbuzzShaper
}
type receiptRow struct {
	text string
	runs shaping.Line
}

func newReceiptText() *receiptText {
	t := &receiptText{}
	for _, name := range receiptFontFiles {
		data, err := receiptFontAssets.ReadFile("receiptfonts/" + name)
		if err != nil {
			panic("missing embedded receipt font")
		}
		face, err := font.ParseTTF(bytes.NewReader(data))
		if err != nil {
			panic("invalid embedded receipt font")
		}
		t.faces = append(t.faces, face)
	}
	return t
}
func (t *receiptText) ResolveFace(r rune) *font.Face {
	for _, f := range t.faces {
		if _, ok := f.NominalGlyph(r); ok {
			return f
		}
	}
	return t.faces[0]
}
func asciiReceipt(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

// layout lets the shaper choose cluster-safe breaks and the Unicode bidi order.
// A separate wrapper per paragraph keeps its returned glyph slices immutable.
func (t *receiptText) layout(s string) []receiptRow {
	runes := []rune(s)
	var segmenter shaping.Segmenter
	input := shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: fixed.I(12)}
	inputs := segmenter.Split(input, t)
	runs := make([]shaping.Output, 0, len(inputs))
	for _, in := range inputs {
		runs = append(runs, t.shaper.Shape(in))
	}
	var wrapper shaping.LineWrapper
	lines, _ := wrapper.WrapParagraph(shaping.WrapConfig{Direction: di.DirectionLTR}, 450, runes, shaping.NewSliceIterator(runs))
	rows := make([]receiptRow, 0, len(lines))
	for _, line := range lines {
		start, end := len(runes), 0
		for _, run := range line {
			start = min(start, run.Runes.Offset)
			end = max(end, run.Runes.Offset+run.Runes.Count)
		}
		if start <= end {
			rows = append(rows, receiptRow{text: string(runes[start:end]), runs: line})
		}
	}
	return rows
}

func (t *receiptText) draw(stream *strings.Builder, row receiptRow, x, y float64) {
	// VisualIndex is supplied by the wrapper after Unicode bidi resolution.
	runs := append(shaping.Line(nil), row.runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].VisualIndex < runs[j].VisualIndex })
	for _, run := range runs {
		pen := x
		scale := 12 / float64(run.Face.Upem())
		for _, glyph := range run.Glyphs {
			outline, ok := run.Face.GlyphDataOutline(glyph.GlyphID)
			if ok && len(outline.Segments) > 0 {
				gx := pen + float64(glyph.XOffset)/64
				gy := y + float64(glyph.YOffset)/64
				fmt.Fprintf(stream, "q %.6f 0 0 %.6f %.4f %.4f cm\n", scale, scale, gx, gy)
				outlinePDF(stream, outline)
				stream.WriteString("f Q\n")
			}
			pen += float64(glyph.Advance) / 64
		}
		x += float64(run.Advance) / 64
	}
}
func outlinePDF(out *strings.Builder, outline font.GlyphOutline) {
	var current ot.SegmentPoint
	open := false
	for _, s := range outline.Segments {
		p := s.Args
		switch s.Op {
		case ot.SegmentOpMoveTo:
			if open {
				out.WriteString("h\n")
			}
			fmt.Fprintf(out, "%.3f %.3f m\n", p[0].X, p[0].Y)
			current = p[0]
			open = true
		case ot.SegmentOpLineTo:
			fmt.Fprintf(out, "%.3f %.3f l\n", p[0].X, p[0].Y)
			current = p[0]
		case ot.SegmentOpQuadTo:
			a := ot.SegmentPoint{X: current.X + (p[0].X-current.X)*2/3, Y: current.Y + (p[0].Y-current.Y)*2/3}
			b := ot.SegmentPoint{X: p[1].X + (p[0].X-p[1].X)*2/3, Y: p[1].Y + (p[0].Y-p[1].Y)*2/3}
			fmt.Fprintf(out, "%.3f %.3f %.3f %.3f %.3f %.3f c\n", a.X, a.Y, b.X, b.Y, p[1].X, p[1].Y)
			current = p[1]
		case ot.SegmentOpCubeTo:
			fmt.Fprintf(out, "%.3f %.3f %.3f %.3f %.3f %.3f c\n", p[0].X, p[0].Y, p[1].X, p[1].Y, p[2].X, p[2].Y)
			current = p[2]
		}
	}
	if open {
		out.WriteString("h\n")
	}
}

// The visible outlines carry the real shaped forms. A searchable logical-text
// layer preserves characters independently of ligatures and visual RTL order,
// as in a searchable scanned PDF. It uses an embedded font and a ToUnicode map.
type receiptUnicode struct {
	ids   map[rune]uint16
	runes []rune
}

func newReceiptUnicode(rows []receiptRow) *receiptUnicode {
	u := &receiptUnicode{ids: map[rune]uint16{}}
	for _, row := range rows {
		if row.runs == nil {
			continue
		}
		for _, r := range row.text {
			if _, ok := u.ids[r]; !ok {
				u.runes = append(u.runes, r)
				u.ids[r] = uint16(len(u.runes))
			}
		}
	}
	return u
}
func (u *receiptUnicode) encode(s string) string {
	s = receiptSearchOrder(s)
	data := make([]byte, 0, 2*len(s))
	for _, r := range s {
		data = binary.BigEndian.AppendUint16(data, u.ids[r])
	}
	return hex.EncodeToString(data)
}

// PDF text consumers recover RTL logical text from visual character order.
// The ToUnicode map retains each original code point, including combining marks.
func receiptSearchOrder(s string) string {
	var p bidi.Paragraph
	if _, err := p.SetString(s, bidi.DefaultDirection(bidi.LeftToRight)); err != nil {
		return s
	}
	order, err := p.Order()
	if err != nil {
		return s
	}
	var out strings.Builder
	for i := 0; i < order.NumRuns(); i++ {
		run := order.Run(i)
		text := []rune(run.String())
		if run.Direction() == bidi.RightToLeft {
			for a, b := 0, len(text)-1; a < b; a, b = a+1, b-1 {
				text[a], text[b] = text[b], text[a]
			}
		}
		out.WriteString(string(text))
	}
	return out.String()
}
func (u *receiptUnicode) fontObjects() []string {
	data, err := receiptFontAssets.ReadFile("receiptfonts/NotoSans-Regular.ttf")
	if err != nil {
		panic("missing embedded receipt font")
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write(data)
	_ = zw.Close()
	// The invisible font may use .notdef where a script-specific face is needed;
	// visible glyphs always come from the appropriate real Noto outline above.
	glyphs := make([]byte, 2*(len(u.runes)+1))
	var widths strings.Builder
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		panic("invalid embedded receipt font")
	}
	for i, r := range u.runes {
		gid, _ := face.NominalGlyph(r)
		binary.BigEndian.PutUint16(glyphs[2*(i+1):], uint16(gid))
		width := 500
		if unicode.IsMark(r) {
			width = 0
		}
		fmt.Fprintf(&widths, "%d ", width)
	}
	var cmap strings.Builder
	cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /SwiftDropUnicode def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for start := 0; start < len(u.runes); start += 100 {
		end := min(start+100, len(u.runes))
		fmt.Fprintf(&cmap, "%d beginbfchar\n", end-start)
		for i := start; i < end; i++ {
			var raw []byte
			for _, n := range utf16.Encode([]rune{u.runes[i]}) {
				raw = binary.BigEndian.AppendUint16(raw, n)
			}
			fmt.Fprintf(&cmap, "<%04X> <%s>\n", i+1, hex.EncodeToString(raw))
		}
		cmap.WriteString("endbfchar\n")
	}
	cmap.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend end\n")
	return []string{
		"<< /Type /Font /Subtype /Type0 /BaseFont /NotoSans-Regular /Encoding /Identity-H /DescendantFonts [5 0 R] /ToUnicode 9 0 R >>",
		fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /NotoSans-Regular /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 6 0 R /DW 500 /W [1 [%s]] /CIDToGIDMap 8 0 R >>", widths.String()),
		"<< /Type /FontDescriptor /FontName /NotoSans-Regular /Flags 32 /FontBBox [-621 -389 2800 1067] /ItalicAngle 0 /Ascent 1069 /Descent -293 /CapHeight 714 /StemV 80 /FontFile2 7 0 R >>",
		fmt.Sprintf("<< /Length %d /Length1 %d /Filter /FlateDecode >>\nstream\n%sendstream", compressed.Len(), len(data), compressed.String()),
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(glyphs), string(glyphs)),
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", cmap.Len(), cmap.String()),
	}
}
func receiptHasUnsupportedRune(t *receiptText, s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || r == '\u200d' || r == '\u200c' {
			continue
		}
		found := false
		for _, f := range t.faces {
			if _, ok := f.NominalGlyph(r); ok {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}
