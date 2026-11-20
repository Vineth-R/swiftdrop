// Package receipt renders a merchant order receipt as a private PDF. The
// content comes from the persisted merchant order snapshot; the handler checks
// that the order belongs to the authenticated merchant before calling Render.
package receipt

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/packages/go-common/receiptdesign"
)

//go:embed fonts/NotoSans-Regular.ttf
var notoRegular []byte

//go:embed fonts/NotoSansSinhala-Regular.ttf
var notoSinhala []byte

//go:embed fonts/NotoSansTamil-Regular.ttf
var notoTamil []byte

// Merchant is the business information printed on the receipt.
type Merchant struct {
	BusinessName  string
	ContactEmail  string
	ContactPhone  string
	PickupAddress string
}

// Input is one merchant order receipt.
type Input struct {
	Merchant     Merchant
	Order        orders.Detail
	SupportEmail string
	GeneratedAt  time.Time
}

type cardField struct {
	label string
	lines []string
}

// Render returns the PDF bytes. It never returns a partially valid PDF: an
// error means the caller should send a generic server error.
func Render(in Input) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(14, 14, 14)
	// Every long value is measured before it is drawn and the body manages its
	// own page breaks. Automatic breaks would split a card or a measured value
	// and cause overlapping content.
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(false)
	pdf.AliasNbPages("{nb}")

	pdf.AddUTF8FontFromBytes("noto", "", notoRegular)
	pdf.AddUTF8FontFromBytes("noto", "S", notoSinhala)
	pdf.AddUTF8FontFromBytes("noto", "T", notoTamil)
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("load receipt fonts: %w", err)
	}

	pdf.AddPage()
	header(pdf, in)
	body(pdf, in)
	footer(pdf, in.SupportEmail)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("write receipt pdf: %w", err)
	}
	return out.Bytes(), nil
}

func header(pdf *fpdf.Fpdf, in Input) {
	pdf.SetFillColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.Rect(0, 0, 210, 42, "F")

	pdf.SetFillColor(receiptdesign.Lime.R, receiptdesign.Lime.G, receiptdesign.Lime.B)
	pdf.RoundedRect(14, 13, 14, 14, 4, "1234", "F")
	pdf.SetTextColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetXY(14, 15.5)
	pdf.CellFormat(14, 8, "S", "", 0, "C", false, 0, "")

	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 17)
	pdf.SetXY(32, 14)
	pdf.CellFormat(80, 8, "SwiftDrop", "", 0, "L", false, 0, "")

	pdf.SetTextColor(receiptdesign.Pale.R, receiptdesign.Pale.G, receiptdesign.Pale.B)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(32, 24)
	pdf.CellFormat(100, 5, "MERCHANT ORDER RECEIPT", "", 0, "L", false, 0, "")

	pdf.SetTextColor(receiptdesign.Pale.R, receiptdesign.Pale.G, receiptdesign.Pale.B)
	pdf.SetXY(120, 12)
	pdf.CellFormat(76, 4, "TRACKING NUMBER", "", 0, "R", false, 0, "")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetXY(110, 17)
	pdf.CellFormat(86, 7, in.Order.TrackingNumber, "", 0, "R", false, 0, "")

	pdf.SetTextColor(receiptdesign.Pale.R, receiptdesign.Pale.G, receiptdesign.Pale.B)
	pdf.SetFont("noto", "", 8)
	pdf.SetXY(110, 27)
	pdf.CellFormat(86, 4, "Issued "+receiptdesign.DateTime(in.GeneratedAt), "", 0, "R", false, 0, "")
}

func slimHeader(pdf *fpdf.Fpdf, in Input) {
	pdf.SetFillColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.Rect(0, 0, 210, 14, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(14, 8)
	pdf.CellFormat(80, 5, "SwiftDrop", "", 0, "L", false, 0, "")
	pdf.SetTextColor(receiptdesign.Pale.R, receiptdesign.Pale.G, receiptdesign.Pale.B)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetXY(60, 8)
	pdf.CellFormat(90, 5, "Merchant order receipt", "", 0, "L", false, 0, "")
	pdf.SetXY(150, 8)
	pdf.CellFormat(46, 5, in.Order.TrackingNumber, "", 0, "R", false, 0, "")
}

func body(pdf *fpdf.Fpdf, in Input) {
	order := in.Order
	y := 50.0
	newPage := func() {
		pdf.AddPage()
		slimHeader(pdf, in)
		y = 22
	}
	ensure := func(needed float64) {
		if y+needed > 275 {
			newPage()
		}
	}

	// External reference, order date and status.
	pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(14, y)
	pdf.CellFormat(40, 4, "EXTERNAL ORDER ID", "", 0, "L", false, 0, "")
	pdf.SetXY(88, y)
	pdf.CellFormat(40, 4, "ORDER DATE", "", 0, "L", false, 0, "")

	externalLines := splitText(pdf, receiptdesign.Fallback(order.ExternalOrderID, "—"), 70, 11)
	pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
	for i, line := range externalLines {
		pdf.SetXY(14, y+5+float64(i)*5)
		pdf.CellFormat(70, 5, line, "", 0, "L", false, 0, "")
	}
	pdf.SetFont("noto", "", 11)
	pdf.SetXY(88, y+5)
	pdf.CellFormat(55, 5, receiptdesign.DateTime(order.CreatedAt), "", 0, "L", false, 0, "")
	status(pdf, 150, y, receiptdesign.StatusLabel(order.Status))

	metaBottom := y + 5 + float64(len(externalLines))*5
	if metaBottom < y+14 {
		metaBottom = y + 14
	}
	y = metaBottom + 8

	// Merchant and recipient cards. Both heights are measured before either
	// background is drawn so long values cannot overflow a fixed card.
	merchantFields := cardFields(pdf, [][2]string{
		{"Business", in.Merchant.BusinessName},
		{"Contact", receiptdesign.JoinContact(in.Merchant.ContactEmail, in.Merchant.ContactPhone)},
		{"Pickup", receiptdesign.JoinContact(in.Merchant.PickupAddress, order.Pickup.City)},
		{"Pickup contact", receiptdesign.JoinContact(order.Pickup.Name, order.Pickup.Phone)},
	}, 59)
	recipientFields := cardFields(pdf, [][2]string{
		{"Name", order.Recipient.Name},
		{"Phone", order.Recipient.Phone},
		{"Address", order.Recipient.Address},
		{"City", order.Recipient.City},
	}, 59)
	cardHeight := max(cardHeightOf(merchantFields), cardHeightOf(recipientFields))
	ensure(cardHeight + 12)
	drawCard(pdf, 14, y, 88, cardHeight, "MERCHANT", merchantFields)
	drawCard(pdf, 108, y, 88, cardHeight, "RECIPIENT", recipientFields)
	y += cardHeight + 12

	// Parcel details.
	ensure(18)
	sectionHeading(pdf, 14, y, "PARCEL")
	y += 7
	pdf.SetDrawColor(receiptdesign.Line.R, receiptdesign.Line.G, receiptdesign.Line.B)
	pdf.Line(14, y, 196, y)
	y += 3
	details := [][2]string{
		{"Description", receiptdesign.Fallback(deref(order.Description), "No description provided")},
		{"Weight", receiptdesign.Decimal(int64(order.WeightKG)) + " kg"},
		{"Size", order.SizeCategory},
		{"Current status", receiptdesign.StatusLabel(order.Status)},
		{"Delivery fee", receiptdesign.Money(int64(order.Fee.Total), order.Fee.Currency)},
	}
	for _, row := range details {
		lines := splitText(pdf, row[1], 144, 9)
		rowHeight := max(6, float64(len(lines))*4.5)
		ensure(rowHeight + 1.5)
		pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetXY(14, y+0.5)
		pdf.CellFormat(38, 4.5, row[0], "", 0, "L", false, 0, "")
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		setNotoFor(pdf, row[1], 9)
		for i, line := range lines {
			pdf.SetXY(52, y+0.5+float64(i)*4.5)
			pdf.CellFormat(144, 4.5, line, "", 0, "L", false, 0, "")
		}
		y += rowHeight + 1.5
	}

	// Delivery fee table.
	ensure(72)
	y += 2
	sectionHeading(pdf, 14, y, "DELIVERY FEE")
	y += 6
	pdf.SetFillColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(14, y)
	pdf.CellFormat(120, 6, "ITEM", "", 0, "L", true, 0, "")
	pdf.SetXY(134, y)
	pdf.CellFormat(62, 6, "AMOUNT ("+order.Fee.Currency+")", "", 0, "R", true, 0, "")
	y += 6
	for _, row := range [][2]string{
		{"Base fee", receiptdesign.Decimal(int64(order.Fee.BaseFee))},
		{"Weight charge", receiptdesign.Decimal(int64(order.Fee.WeightCharge))},
		{"Size surcharge", receiptdesign.Decimal(int64(order.Fee.SizeSurcharge))},
	} {
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		pdf.SetFont("noto", "", 9)
		pdf.SetXY(14, y)
		pdf.CellFormat(120, 6, row[0], "B", 0, "L", false, 0, "")
		pdf.SetXY(134, y)
		pdf.CellFormat(62, 6, row[1], "B", 0, "R", false, 0, "")
		y += 6
	}
	pdf.SetFillColor(receiptdesign.Lime.R, receiptdesign.Lime.G, receiptdesign.Lime.B)
	pdf.SetTextColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(14, y)
	pdf.CellFormat(120, 8, "TOTAL", "", 0, "L", true, 0, "")
	pdf.SetXY(134, y)
	pdf.CellFormat(62, 8, receiptdesign.Money(int64(order.Fee.Total), order.Fee.Currency), "", 0, "R", true, 0, "")
	y += 10

	pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
	pdf.SetFont("noto", "", 8)
	note := "This is a delivery fee snapshot for the order shown above, not a payment or settlement receipt. The fee was calculated when the order was created."
	for _, line := range splitText(pdf, note, 182, 8) {
		pdf.SetXY(14, y)
		pdf.CellFormat(182, 3.5, line, "", 0, "L", false, 0, "")
		y += 3.5
	}

	// Status timeline.
	y += 8
	ensure(24)
	sectionHeading(pdf, 14, y, "STATUS TIMELINE")
	y += 7
	if len(order.Timeline) == 0 {
		pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
		pdf.SetFont("noto", "", 9)
		pdf.SetXY(14, y)
		pdf.CellFormat(182, 6, "No status events recorded yet.", "", 0, "L", false, 0, "")
		return
	}
	start := 0
	if len(order.Timeline) > 10 {
		start = len(order.Timeline) - 10
	}
	for _, event := range order.Timeline[start:] {
		ensure(5)
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetXY(14, y)
		pdf.CellFormat(70, 5, receiptdesign.StatusLabel(event.Status), "B", 0, "L", false, 0, "")
		pdf.SetFont("noto", "", 8)
		pdf.SetXY(90, y)
		pdf.CellFormat(106, 5, receiptdesign.DateTime(event.At), "B", 0, "R", false, 0, "")
		y += 5
	}
}

func status(pdf *fpdf.Fpdf, x, y float64, value string) {
	pdf.SetFont("Helvetica", "B", 8)
	width := pdf.GetStringWidth(value) + 12
	if width < 26 {
		width = 26
	}
	pdf.SetFillColor(receiptdesign.Lime.R, receiptdesign.Lime.G, receiptdesign.Lime.B)
	pdf.RoundedRect(x, y, width, 9, 4, "1234", "F")
	pdf.SetTextColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.SetXY(x, y+2)
	pdf.CellFormat(width, 5, value, "", 0, "C", false, 0, "")
}

func sectionHeading(pdf *fpdf.Fpdf, x, y float64, text string) {
	pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetXY(x, y)
	pdf.CellFormat(182, 5, text, "", 0, "L", false, 0, "")
}

func cardFields(pdf *fpdf.Fpdf, rows [][2]string, width float64) []cardField {
	fields := make([]cardField, 0, len(rows))
	for _, row := range rows {
		fields = append(fields, cardField{label: row[0], lines: splitText(pdf, row[1], width, 8)})
	}
	return fields
}

func cardHeightOf(fields []cardField) float64 {
	height := 7.0
	for _, field := range fields {
		height += 3.5 + float64(len(field.lines))*3.5 + 1.5
	}
	return height
}

func drawCard(pdf *fpdf.Fpdf, x, y, width, height float64, title string, fields []cardField) {
	pdf.SetFillColor(receiptdesign.Surface.R, receiptdesign.Surface.G, receiptdesign.Surface.B)
	pdf.RoundedRect(x, y, width, height, 3, "1234", "F")
	pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(x+5, y+3.5)
	pdf.CellFormat(width-10, 4, title, "", 0, "L", false, 0, "")
	cursor := y + 9
	for _, field := range fields {
		pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetXY(x+5, cursor)
		pdf.CellFormat(20, 3.5, field.label, "", 0, "L", false, 0, "")
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		setNotoFor(pdf, strings.Join(field.lines, ""), 8)
		for i, line := range field.lines {
			pdf.SetXY(x+24, cursor+float64(i)*3.5)
			pdf.CellFormat(width-29, 3.5, line, "", 0, "L", false, 0, "")
		}
		cursor += 3.5 + float64(len(field.lines))*3.5 + 1.5
	}
}

func splitText(pdf *fpdf.Fpdf, value string, width, size float64) []string {
	setNotoFor(pdf, value, size)
	lines := pdf.SplitText(receiptdesign.Fallback(strings.TrimSpace(value), "—"), width)
	if len(lines) == 0 {
		return []string{"—"}
	}
	return lines
}

func footer(pdf *fpdf.Fpdf, supportEmail string) {
	pdf.SetFooterFunc(func() {
		pdf.SetY(-16)
		pdf.SetDrawColor(receiptdesign.Line.R, receiptdesign.Line.G, receiptdesign.Line.B)
		pdf.Line(14, pdf.GetY(), 196, pdf.GetY())
		pdf.SetY(-13)
		pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
		pdf.SetFont("Helvetica", "", 7)
		left := "SwiftDrop | merchant order receipt"
		if supportEmail != "" {
			left += " | " + supportEmail
		}
		pdf.CellFormat(150, 5, left, "", 0, "L", false, 0, "")
		pdf.CellFormat(32, 5, fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
}

// setNotoFor selects the embedded Noto face that covers the value. Sinhala and
// Tamil need their own fonts; Noto Sans Regular covers Latin, Greek and
// Cyrillic. The receipt never mixes scripts inside one cell.
func setNotoFor(pdf *fpdf.Fpdf, value string, size float64) {
	style := ""
	for _, r := range value {
		switch {
		case r >= 0x0D80 && r <= 0x0DFF:
			style = "S"
		case r >= 0x0B80 && r <= 0x0BFF:
			style = "T"
		}
		if style != "" {
			break
		}
	}
	pdf.SetFont("noto", style, size)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
