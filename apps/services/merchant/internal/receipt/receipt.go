// Package receipt renders a merchant order receipt as a private PDF. The
// content comes from the persisted merchant order snapshot; the handler checks
// that the order belongs to the authenticated merchant before calling Render.
package receipt

import (
	"bytes"
	_ "embed"
	"fmt"
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

// Render returns the PDF bytes. It never returns a partially valid PDF: an
// error means the caller should send a generic server error.
func Render(in Input) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(14, 14, 14)
	pdf.SetAutoPageBreak(true, 20)
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

	pdf.SetTextColor(190, 200, 215)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(32, 24)
	pdf.CellFormat(100, 5, "MERCHANT ORDER RECEIPT", "", 0, "L", false, 0, "")

	pdf.SetTextColor(190, 200, 215)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(120, 12)
	pdf.CellFormat(76, 4, "TRACKING NUMBER", "", 0, "R", false, 0, "")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetXY(110, 17)
	pdf.CellFormat(86, 7, in.Order.TrackingNumber, "", 0, "R", false, 0, "")

	pdf.SetTextColor(190, 200, 215)
	pdf.SetFont("noto", "", 8)
	pdf.SetXY(110, 27)
	pdf.CellFormat(86, 4, "Issued "+in.GeneratedAt.UTC().Format("02 Jan 2006 15:04 UTC"), "", 0, "R", false, 0, "")
	pdf.SetY(50)
}

func body(pdf *fpdf.Fpdf, in Input) {
	order := in.Order

	// Identity and status row.
	pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(14, 50)
	pdf.CellFormat(40, 4, "EXTERNAL ORDER ID", "", 0, "L", false, 0, "")
	pdf.SetXY(14, 55)
	pdf.SetFont("noto", "", 11)
	pdf.MultiCell(70, 5, receiptdesign.Fallback(order.ExternalOrderID, "—"), "", "L", false)

	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(88, 50)
	pdf.CellFormat(40, 4, "ORDER DATE", "", 0, "L", false, 0, "")
	pdf.SetXY(88, 55)
	pdf.SetFont("noto", "", 11)
	pdf.MultiCell(55, 5, order.CreatedAt.UTC().Format("02 Jan 2006 15:04 UTC"), "", "L", false)

	status(pdf, 150, 50, receiptdesign.StatusLabel(order.Status))

	// Merchant and recipient cards.
	cardTop := 70.0
	pdf.SetFillColor(receiptdesign.Surface.R, receiptdesign.Surface.G, receiptdesign.Surface.B)
	pdf.RoundedRect(14, cardTop, 88, 52, 3, "1234", "F")
	pdf.RoundedRect(108, cardTop, 88, 52, 3, "1234", "F")

	cardHeading(pdf, 19, cardTop+5, "MERCHANT")
	cardLine(pdf, 19, cardTop+12, "Business", in.Merchant.BusinessName)
	cardLine(pdf, 19, cardTop+20, "Contact", receiptdesign.JoinContact(in.Merchant.ContactEmail, in.Merchant.ContactPhone))
	cardLine(pdf, 19, cardTop+28, "Pickup", receiptdesign.JoinContact(in.Merchant.PickupAddress, order.Pickup.City))
	cardLine(pdf, 19, cardTop+36, "Pickup contact", receiptdesign.JoinContact(order.Pickup.Name, order.Pickup.Phone))

	cardHeading(pdf, 113, cardTop+5, "RECIPIENT")
	cardLine(pdf, 113, cardTop+12, "Name", order.Recipient.Name)
	cardLine(pdf, 113, cardTop+20, "Phone", order.Recipient.Phone)
	cardLine(pdf, 113, cardTop+28, "Address", order.Recipient.Address)
	cardLine(pdf, 113, cardTop+36, "City", order.Recipient.City)

	// Parcel details.
	y := 130.0
	sectionHeading(pdf, 14, y, "PARCEL")
	y += 7
	pdf.SetDrawColor(receiptdesign.Line.R, receiptdesign.Line.G, receiptdesign.Line.B)
	pdf.Line(14, y, 196, y)
	y += 2
	details := [][2]string{
		{"Description", receiptdesign.Fallback(deref(order.Description), "No description provided")},
		{"Weight", receiptdesign.Decimal(int64(order.WeightKG)) + " kg"},
		{"Size", order.SizeCategory},
		{"Current status", receiptdesign.StatusLabel(order.Status)},
		{"Delivery fee", receiptdesign.Money(int64(order.Fee.Total), order.Fee.Currency)},
	}
	y = keyValues(pdf, 14, y, 182, details)

	// Fee breakdown.
	y = 178.0
	sectionHeading(pdf, 14, y, "DELIVERY FEE")
	y += 6
	pdf.SetFillColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(14, y)
	pdf.CellFormat(120, 7, "ITEM", "", 0, "L", true, 0, "")
	pdf.SetXY(134, y)
	pdf.CellFormat(62, 7, "AMOUNT ("+order.Fee.Currency+")", "", 0, "R", true, 0, "")
	y += 7
	feeRows := [][2]string{
		{"Base fee", order.Fee.BaseFee.String()},
		{"Weight charge", order.Fee.WeightCharge.String()},
		{"Size surcharge", order.Fee.SizeSurcharge.String()},
	}
	for _, row := range feeRows {
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		pdf.SetFont("noto", "", 9)
		pdf.SetXY(14, y)
		pdf.CellFormat(120, 7, row[0], "B", 0, "L", false, 0, "")
		pdf.SetXY(134, y)
		pdf.CellFormat(62, 7, row[1], "B", 0, "R", false, 0, "")
		y += 7
	}
	pdf.SetFillColor(receiptdesign.Lime.R, receiptdesign.Lime.G, receiptdesign.Lime.B)
	pdf.SetTextColor(receiptdesign.Navy.R, receiptdesign.Navy.G, receiptdesign.Navy.B)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(14, y)
	pdf.CellFormat(120, 9, "TOTAL", "", 0, "L", true, 0, "")
	pdf.SetXY(134, y)
	pdf.CellFormat(62, 9, receiptdesign.Money(int64(order.Fee.Total), order.Fee.Currency), "", 0, "R", true, 0, "")
	y += 12

	pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
	pdf.SetFont("noto", "", 8)
	pdf.SetXY(14, y)
	pdf.MultiCell(182, 4, "This is a delivery fee snapshot for the order shown above, not a payment or settlement receipt. The fee was calculated when the order was created.", "", "L", false)

	// Timeline.
	y += 10
	if y > 245 {
		pdf.AddPage()
		y = 20
	}
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
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetXY(14, y)
		pdf.CellFormat(70, 6, event.Status, "B", 0, "L", false, 0, "")
		pdf.SetFont("noto", "", 8)
		pdf.SetXY(90, y)
		pdf.CellFormat(106, 6, event.At.UTC().Format("02 Jan 2006 15:04 UTC"), "B", 0, "R", false, 0, "")
		y += 6
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

func cardHeading(pdf *fpdf.Fpdf, x, y float64, text string) {
	pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(x, y)
	pdf.CellFormat(78, 4, text, "", 0, "L", false, 0, "")
}

func cardLine(pdf *fpdf.Fpdf, x, y float64, label, value string) {
	pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(x, y)
	pdf.CellFormat(22, 4, label, "", 0, "L", false, 0, "")
	pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
	setNotoFor(pdf, value, 8)
	pdf.SetXY(x+22, y)
	pdf.MultiCell(61, 4, receiptdesign.Fallback(value, "—"), "", "L", false)
}

func keyValues(pdf *fpdf.Fpdf, x, y, width float64, rows [][2]string) float64 {
	for _, row := range rows {
		pdf.SetTextColor(receiptdesign.Muted.R, receiptdesign.Muted.G, receiptdesign.Muted.B)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetXY(x, y)
		pdf.CellFormat(38, 7, row[0], "", 0, "L", false, 0, "")
		pdf.SetTextColor(receiptdesign.Ink.R, receiptdesign.Ink.G, receiptdesign.Ink.B)
		setNotoFor(pdf, row[1], 9)
		pdf.SetXY(x+38, y)
		pdf.MultiCell(width-38, 5, receiptdesign.Fallback(row[1], "—"), "", "L", false)
		y += 7
	}
	return y
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
