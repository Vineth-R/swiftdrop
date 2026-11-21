package emails

import "fmt"

// ParcelStatus is a parcel's status as the API names it (API 4.x, 6.3).
type ParcelStatus string

const (
	ParcelCreated        ParcelStatus = "CREATED"
	ParcelPickedUp       ParcelStatus = "PICKED_UP"
	ParcelInTransit      ParcelStatus = "IN_TRANSIT"
	ParcelOutForDelivery ParcelStatus = "OUT_FOR_DELIVERY"
	ParcelDelivered      ParcelStatus = "DELIVERED"
	ParcelFailed         ParcelStatus = "FAILED"
)

// ParcelData fills ParcelUpdate.
type ParcelData struct {
	// Name is who gets the email, usually the customer or merchant who sent it.
	Name           string
	TrackingNumber string
	// Origin and Destination are cities.
	Origin      string
	Destination string
	// When is the formatted time of the update (see FormatTime).
	When string
	// Note is the failure reason for FAILED and who received the parcel for
	// DELIVERED. Other statuses ignore it.
	Note string
	// TrackURL is the absolute link to follow the parcel.
	TrackURL string
}

// ParcelUpdate is sent when a parcel reaches a new status. One layout, six
// wordings; an unknown status is an error, so a typo cannot send a blank email.
func ParcelUpdate(status ParcelStatus, d ParcelData) (Email, error) {
	var (
		subject, heading, lead string
		notice                 *Block
	)
	switch status {
	case ParcelCreated:
		subject = fmt.Sprintf("Your parcel %s is booked", d.TrackingNumber)
		heading = "We have your parcel"
		lead = fmt.Sprintf("Your parcel from %s to %s is booked. We will email you at each step of the way.", d.Origin, d.Destination)
	case ParcelPickedUp:
		subject = fmt.Sprintf("Your parcel %s was picked up", d.TrackingNumber)
		heading = "Your parcel was picked up"
		lead = fmt.Sprintf("A courier collected your parcel in %s. It is now on its way to %s.", d.Origin, d.Destination)
	case ParcelInTransit:
		subject = fmt.Sprintf("Your parcel %s is on its way", d.TrackingNumber)
		heading = "Your parcel is on its way"
		lead = fmt.Sprintf("Your parcel is moving through our network towards %s.", d.Destination)
	case ParcelOutForDelivery:
		subject = fmt.Sprintf("Your parcel %s is out for delivery", d.TrackingNumber)
		heading = "Your parcel is out for delivery"
		lead = fmt.Sprintf("A courier is on the way to %s with your parcel.", d.Destination)
	case ParcelDelivered:
		subject = fmt.Sprintf("Your parcel %s was delivered", d.TrackingNumber)
		heading = "Your parcel was delivered"
		lead = fmt.Sprintf("Your parcel reached %s.", d.Destination)
		text := "Delivered on " + d.When + "."
		if d.Note != "" {
			text = "Received by " + d.Note + " on " + d.When + "."
		}
		block := Notice(ToneSuccess, "Delivered", text)
		notice = &block
	case ParcelFailed:
		subject = fmt.Sprintf("We could not deliver parcel %s", d.TrackingNumber)
		heading = "We could not deliver your parcel"
		lead = fmt.Sprintf("A courier tried to deliver your parcel in %s but could not complete the delivery.", d.Destination)
		reason := d.Note
		if reason == "" {
			reason = "The courier could not complete the delivery."
		}
		block := Notice(ToneDanger, "What happened", reason)
		notice = &block
	default:
		return Email{}, fmt.Errorf("emails: %q is not a parcel status", status)
	}

	blocks := []Block{Paragraph(greeting(d.Name)), Paragraph(lead)}
	if notice != nil {
		blocks = append(blocks, *notice)
	}
	blocks = append(blocks,
		Details(
			Row{Label: "Tracking number", Value: d.TrackingNumber, IsMono: true},
			Row{Label: "From", Value: d.Origin},
			Row{Label: "To", Value: d.Destination},
			Row{Label: "Updated", Value: d.When},
		),
		Button("Track your parcel", d.TrackURL),
	)
	if status == ParcelFailed {
		blocks = append(blocks, Paragraph("Reply to this email or contact SwiftDrop support to talk about what to do next."))
	}

	return Email{
		Subject:   subject,
		Preheader: heading + ": " + d.TrackingNumber + ", " + d.Origin + " to " + d.Destination + ".",
		Eyebrow:   "Parcel update",
		Heading:   heading,
		Blocks:    blocks,
		Reason:    "You are receiving this because you sent parcel " + d.TrackingNumber + " with SwiftDrop.",
	}, nil
}
