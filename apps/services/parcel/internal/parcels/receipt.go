package parcels

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Vineth-R/swiftdrop/packages/go-common/receiptdesign"
)

type rowKind int

const (
	rowSection rowKind = iota
	rowLabel
	rowValue
	rowAmount
	rowTotal
	rowTimeline
)

// Receipt produces a searchable PDF from authorized, stored parcel values.
// Unicode names use correctly shaped Noto outlines and a logical text layer.
// It returns an error when the bundled font resources cannot be initialized;
// the caller turns that into a generic server error.
func Receipt(p Detail) ([]byte, error) {
	return ReceiptWithHistory(p, nil, "")
}

// ReceiptWithHistory is the customer parcel receipt. It adds the status
// timeline and the configured support address. The customer receipt keeps the
// project's shaping-capable text engine because fpdf cannot shape Sinhala,
// Tamil, Arabic or Hebrew; both receipts share receiptdesign for branding,
// typography, spacing and formatting.
func ReceiptWithHistory(p Detail, history []HistoryItem, supportEmail string) ([]byte, error) {
	needsShaping := false
	for _, value := range receiptValues(p, history) {
		if !asciiReceipt(value) {
			needsShaping = true
			break
		}
	}
	var text *receiptText
	if needsShaping {
		// The parsed faces carry mutable glyph caches. Hold the lock while the
		// rows are laid out and drawn so concurrent receipts cannot corrupt it.
		receiptRenderMu.Lock()
		defer receiptRenderMu.Unlock()
		var err error
		text, err = newReceiptText()
		if err != nil {
			return nil, err
		}
	}

	rows := customerRows(p, history, text)
	var logical *receiptUnicode
	if text != nil {
		logical = newReceiptUnicode(rows)
	}
	pages := buildReceiptPages(p, rows, text, logical, supportEmail, time.Now().UTC())

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	if text != nil {
		objects = append(objects, logical.fontObjects(text.fonts)...)
	}
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")
	boldObject := len(objects)

	firstPage := len(objects) + 1
	var kids strings.Builder
	for i := range pages {
		fmt.Fprintf(&kids, "%d 0 R ", firstPage+2*i)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pages))

	fonts := "/F1 3 0 R /F3 " + strconv.Itoa(boldObject) + " 0 R"
	if text != nil {
		fonts += " /F2 4 0 R"
	}
	for i, stream := range pages {
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << %s >> >> /Contents %d 0 R >>", fonts, firstPage+1+2*i),
			fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		)
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes(), nil
}

func receiptValues(p Detail, history []HistoryItem) []string {
	values := []string{
		p.TrackingNumber, p.Sender.Name, p.Sender.Phone, p.Sender.Address, p.Sender.City,
		p.Recipient.Name, p.Recipient.Phone, p.Recipient.Address, p.Recipient.City,
		p.SizeCategory, p.Status, p.Fee.Currency,
	}
	if p.Description != nil {
		values = append(values, *p.Description)
	}
	for _, event := range history {
		values = append(values, event.Status)
		if event.Note != nil {
			values = append(values, *event.Note)
		}
		if event.FailureReason != nil {
			values = append(values, *event.FailureReason)
		}
	}
	return values
}

func customerRows(p Detail, history []HistoryItem, text *receiptText) []receiptRow {
	rows := make([]receiptRow, 0, 64)
	addSection := func(title string) {
		rows = append(rows, receiptRow{text: title, kind: rowSection, height: 28})
	}
	addLabel := func(label string) {
		rows = append(rows, receiptRow{text: label, kind: rowLabel, height: 14})
	}
	addValue := func(value string) {
		value = receiptdesign.Fallback(value, "—")
		if text != nil && !asciiReceipt(value) {
			for _, row := range text.layoutWidth(value, 450) {
				row.kind, row.height = rowValue, 18
				rows = append(rows, row)
			}
			return
		}
		for _, line := range receiptLines(value) {
			rows = append(rows, receiptRow{text: line, kind: rowValue, height: 18})
		}
	}
	addField := func(label, value string) {
		addLabel(label)
		addValue(value)
	}
	addAmount := func(label, value string, total bool) {
		kind, height := rowAmount, 20.0
		if total {
			kind, height = rowTotal, 28.0
		}
		rows = append(rows, receiptRow{text: label, right: value, kind: kind, height: height})
	}
	addTimeline := func(status, at string) {
		rows = append(rows, receiptRow{text: status, right: at, kind: rowTimeline, height: 18})
	}

	addSection("PARCEL")
	addField("Tracking number", p.TrackingNumber)
	addField("Status", receiptdesign.StatusLabel(p.Status))
	addField("Created", receiptdesign.DateTime(p.CreatedAt))
	addField("Updated", receiptdesign.DateTime(p.UpdatedAt))
	if p.DeliveredAt != nil {
		addField("Delivered", receiptdesign.DateTime(*p.DeliveredAt))
	}

	addSection("SENDER")
	addField("Name", p.Sender.Name)
	addField("Phone", p.Sender.Phone)
	addField("Pickup address", p.Sender.Address)
	addField("Pickup city", p.Sender.City)

	addSection("RECIPIENT")
	addField("Name", p.Recipient.Name)
	addField("Phone", p.Recipient.Phone)
	addField("Delivery address", p.Recipient.Address)
	addField("Destination city", p.Recipient.City)

	addSection("PARCEL DETAILS")
	description := "No description provided"
	if p.Description != nil && strings.TrimSpace(*p.Description) != "" {
		description = *p.Description
	}
	addField("Description", description)
	addField("Weight", receiptdesign.Decimal(int64(p.WeightKG))+" kg")
	addField("Size", p.SizeCategory)

	addSection("DELIVERY FEE")
	addAmount("Base fee", receiptdesign.Money(int64(p.Fee.BaseFee), p.Fee.Currency), false)
	addAmount("Weight charge", receiptdesign.Money(int64(p.Fee.WeightCharge), p.Fee.Currency), false)
	addAmount("Size surcharge", receiptdesign.Money(int64(p.Fee.SizeSurcharge), p.Fee.Currency), false)
	addAmount("Total", receiptdesign.Money(int64(p.Fee.Total), p.Fee.Currency), true)

	addSection("STATUS HISTORY")
	if len(history) == 0 {
		addValue("No status events recorded yet.")
	} else {
		for _, event := range history {
			addTimeline(receiptdesign.StatusLabel(event.Status), receiptdesign.DateTime(event.ChangedAt))
		}
	}
	return rows
}

func buildReceiptPages(p Detail, rows []receiptRow, text *receiptText, logical *receiptUnicode, supportEmail string, issuedAt time.Time) []string {
	pages := make([]string, 0, 2)
	var current strings.Builder
	first := true
	y := 0.0
	newPage := func() {
		drawPageChrome(&current, p, first, supportEmail, issuedAt)
		if first {
			y = 700
		} else {
			y = 790
		}
		first = false
	}
	newPage()
	for _, row := range rows {
		needed := row.height
		if row.kind == rowSection {
			switch row.text {
			case "DELIVERY FEE":
				needed = 140
			case "STATUS HISTORY":
				needed = 120
			}
		}
		if y-needed < 60 {
			pages = append(pages, current.String())
			current.Reset()
			newPage()
		}
		drawReceiptRow(&current, row, 40, y, text, logical)
		y -= row.height
	}
	pages = append(pages, current.String())
	total := strconv.Itoa(len(pages))
	for i := range pages {
		pages[i] = strings.ReplaceAll(pages[i], "{nb}", total)
		pages[i] = strings.ReplaceAll(pages[i], "{page}", strconv.Itoa(i+1))
	}
	return pages
}

func drawPageChrome(stream *strings.Builder, p Detail, first bool, supportEmail string, issuedAt time.Time) {
	pageWidth := receiptdesign.MMToPoints(receiptdesign.PageWidthMM)
	pageHeight := receiptdesign.MMToPoints(receiptdesign.PageHeightMM)
	if first {
		fillRect(stream, receiptdesign.Navy, 0, pageHeight-120, pageWidth, 120)
		fillRect(stream, receiptdesign.Lime, 40, pageHeight-90, 40, 40)
		setTextColor(stream, receiptdesign.Navy)
		drawASCII(stream, "/F3", 24, 52, pageHeight-70, "S")
		setTextColor(stream, receiptdesign.White)
		drawASCII(stream, "/F3", 22, 95, pageHeight-62, "SwiftDrop")
		setTextColor(stream, receiptdesign.Pale)
		drawASCII(stream, "/F3", 8, 95, pageHeight-84, "PARCEL DELIVERY RECEIPT")
		drawASCII(stream, "/F1", 8, 380, pageHeight-32, "TRACKING NUMBER")
		drawASCII(stream, "/F3", 14, 380, pageHeight-52, p.TrackingNumber)
		drawASCII(stream, "/F1", 8, 380, pageHeight-90, "Issued "+receiptdesign.DateTime(issuedAt))
	} else {
		fillRect(stream, receiptdesign.Navy, 0, pageHeight-48, pageWidth, 48)
		setTextColor(stream, receiptdesign.White)
		drawASCII(stream, "/F3", 14, 40, pageHeight-30, "SwiftDrop")
		setTextColor(stream, receiptdesign.Pale)
		drawASCII(stream, "/F1", 8, 130, pageHeight-30, "Parcel Delivery Receipt")
		drawASCII(stream, "/F1", 8, 430, pageHeight-30, p.TrackingNumber)
	}
	setTextColor(stream, receiptdesign.Muted)
	fmt.Fprintf(stream, "q %.3f %.3f %.3f RG 0.5 w 40 42 m 555 42 l S Q\n",
		float64(receiptdesign.Line.R)/255, float64(receiptdesign.Line.G)/255, float64(receiptdesign.Line.B)/255)
	drawASCII(stream, "/F1", 8, 40, 28, "SwiftDrop | Parcel Delivery Receipt")
	if supportEmail != "" {
		drawASCII(stream, "/F1", 8, 40, 38, supportEmail)
	}
	drawASCII(stream, "/F1", 8, 500, 28, "Page {page} of {nb}")
}

func drawReceiptRow(stream *strings.Builder, row receiptRow, x, y float64, text *receiptText, logical *receiptUnicode) {
	switch row.kind {
	case rowSection:
		fillRect(stream, receiptdesign.Surface, 40, y-22, 515, 26)
		setTextColor(stream, receiptdesign.Navy)
		drawASCII(stream, "/F3", 10, 48, y-6, row.text)
	case rowLabel:
		setTextColor(stream, receiptdesign.Muted)
		drawASCII(stream, "/F3", 8, 48, y-10, strings.ToUpper(row.text))
	case rowValue:
		setTextColor(stream, receiptdesign.Ink)
		drawTextRow(stream, row, 48, y-12, text, logical)
	case rowAmount:
		setTextColor(stream, receiptdesign.Ink)
		drawASCII(stream, "/F1", 11, 48, y-13, row.text)
		drawRight(stream, "/F1", 11, 508, y-13, row.right)
	case rowTotal:
		fillRect(stream, receiptdesign.Lime, 40, y-26, 515, 28)
		setTextColor(stream, receiptdesign.Navy)
		drawASCII(stream, "/F3", 11, 48, y-16, row.text)
		drawRight(stream, "/F3", 11, 508, y-16, row.right)
	case rowTimeline:
		setTextColor(stream, receiptdesign.Ink)
		drawASCII(stream, "/F1", 10, 48, y-12, row.text)
		drawRight(stream, "/F1", 10, 508, y-12, row.right)
	}
}

func drawTextRow(stream *strings.Builder, row receiptRow, x, y float64, text *receiptText, logical *receiptUnicode) {
	if row.runs == nil {
		drawASCII(stream, "/F1", 11, x, y, row.text)
		return
	}
	drawUnicodeRow(stream, row, x, y, text, logical)
}

func drawUnicodeRow(stream *strings.Builder, row receiptRow, x, y float64, text *receiptText, logical *receiptUnicode) {
	utf := []byte{0xfe, 0xff}
	for _, n := range utf16.Encode([]rune(row.text)) {
		utf = append(utf, byte(n>>8), byte(n))
	}
	fmt.Fprintf(stream, "/Span << /ActualText <%s> >> BDC\n", hex.EncodeToString(utf))
	text.draw(stream, row, x, y)
	fmt.Fprintf(stream, "BT /F2 12 Tf 3 Tr 1 0 0 1 %.3f %.3f Tm <%s> Tj ET\n", x, y, logical.encode(row.text))
	stream.WriteString("EMC\n")
}

func drawASCII(stream *strings.Builder, font string, size, x, y float64, value string) {
	fmt.Fprintf(stream, "BT %s %.3f Tf 0 Tr 1 0 0 1 %.3f %.3f Tm (%s) Tj ET\n", font, size, x, y, pdfEscape(value))
}

func drawRight(stream *strings.Builder, font string, size, right, y float64, value string) {
	width := float64(len([]rune(value))) * size * 0.52
	drawASCII(stream, font, size, right-width, y, value)
}

func fillRect(stream *strings.Builder, color receiptdesign.RGB, x, y, width, height float64) {
	fmt.Fprintf(stream, "q %.3f %.3f %.3f rg %.3f %.3f %.3f %.3f re f Q\n",
		float64(color.R)/255, float64(color.G)/255, float64(color.B)/255, x, y, width, height)
}

func setTextColor(stream *strings.Builder, color receiptdesign.RGB) {
	fmt.Fprintf(stream, "%.3f %.3f %.3f rg\n", float64(color.R)/255, float64(color.G)/255, float64(color.B)/255)
}

// At most 40 characters fit the content width even at Helvetica's widest
// ordinary Latin glyph. Longer receipts continue on another page.
func receiptLines(s string) []string {
	const width = 40
	words := strings.Fields(s)
	out := []string{}
	line := ""
	for _, word := range words {
		runes := []rune(word)
		for len(runes) > width {
			if line != "" {
				out = append(out, line)
				line = ""
			}
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		word = string(runes)
		if word == "" {
			continue
		}
		switch {
		case line == "":
			line = word
		case len([]rune(line))+1+len(runes) > width:
			out = append(out, line)
			line = word
		default:
			line += " " + word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

func pdfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r >= 32 && r <= 126:
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}
