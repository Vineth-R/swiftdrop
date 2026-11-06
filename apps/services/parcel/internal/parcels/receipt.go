package parcels

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Receipt produces a searchable PDF from authorized, stored parcel values.
// Unicode names use correctly shaped Noto outlines and a logical text layer.
func Receipt(p Detail) []byte {
	lines := []string{"SwiftDrop delivery receipt", "Tracking number: " + p.TrackingNumber, "Sender: " + p.Sender.Name, "Pickup city: " + p.Sender.City, "Recipient: " + p.Recipient.Name, "Destination city: " + p.Recipient.City, "Weight (kg): " + p.WeightKG.String(), "Size: " + p.SizeCategory, "Status: " + p.Status, "Created: " + p.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), "Updated: " + p.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), "Currency: " + p.Fee.Currency, "Base fee: " + p.Fee.BaseFee.String(), "Weight charge: " + p.Fee.WeightCharge.String(), "Size surcharge: " + p.Fee.SizeSurcharge.String(), "Total fee: " + p.Fee.Total.String()}
	if p.DeliveredAt != nil {
		lines = append(lines, "Delivered: "+p.DeliveredAt.UTC().Format("2006-01-02 15:04:05 UTC"))
	}
	rows := make([]receiptRow, 0, len(lines))
	var text *receiptText
	for _, line := range lines {
		if asciiReceipt(line) {
			for _, part := range receiptLines(line) {
				rows = append(rows, receiptRow{text: part})
			}
		} else {
			if text == nil {
				text = newReceiptText()
			}
			rows = append(rows, text.layout(line)...)
		}
	}
	const linesPerPage = 32
	pageCount := (len(rows) + linesPerPage - 1) / linesPerPage
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	var logical *receiptUnicode
	if text != nil {
		logical = newReceiptUnicode(rows)
		objects = append(objects, logical.fontObjects()...)
	}
	firstPage := len(objects) + 1
	var kids strings.Builder
	for i := 0; i < pageCount; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", firstPage+2*i)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), pageCount)
	for page := 0; page < pageCount; page++ {
		var stream strings.Builder
		end := min((page+1)*linesPerPage, len(rows))
		for i, row := range rows[page*linesPerPage : end] {
			line := row.text
			y := 790 - float64(i)*22
			utf := []byte{0xfe, 0xff}
			for _, n := range utf16.Encode([]rune(line)) {
				utf = append(utf, byte(n>>8), byte(n))
			}
			if row.runs == nil {
				fmt.Fprintf(&stream, "/Span << /ActualText <%s> >> BDC\n", hex.EncodeToString(utf))
			} else {
				stream.WriteString("/Span BMC\n")
			}
			if row.runs == nil {
				fmt.Fprintf(&stream, "BT /F1 12 Tf 0 Tr 1 0 0 1 48 %.3f Tm (%s) Tj ET\n", y, pdfEscape(line))
			} else {
				text.draw(&stream, row, 48, y)
				size := min(12.0, 900/float64(max(1, len([]rune(line)))))
				fmt.Fprintf(&stream, "BT /F2 %.3f Tf 3 Tr 1 0 0 1 48 %.3f Tm <%s> Tj ET\n", size, y, logical.encode(line))
			}
			stream.WriteString("EMC\n")
		}
		fonts := "/F1 3 0 R"
		if logical != nil {
			fonts += " /F2 4 0 R"
		}
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << %s >> >> /Contents %d 0 R >>", fonts, firstPage+1+2*page), fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", stream.Len(), stream.String()))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
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
		if len([]rune(line))+1+len(runes) > width {
			out = append(out, line)
			line = word
		} else if line == "" {
			line = word
		} else {
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
