package dispatch

import (
	"errors"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

// Domain errors mapped to API v1.1 status codes.
var (
	ErrParcelNotFound     = errors.New("parcel not found")
	ErrCourierNotEligible = errors.New("courier is not eligible for assignment")
	ErrServiceUnavailable = errors.New("dependent service is unavailable")
	ErrParcelClosed       = statemachine.ErrParcelClosed
	ErrInvalidInput       = errors.New("invalid input")
)

// UnassignedParcel represents an unassigned parcel item in API 5.1.
type UnassignedParcel struct {
	ID             string    `json:"id"`
	TrackingNumber string    `json:"trackingNumber"`
	Status         string    `json:"status"`
	PickupAddress  string    `json:"pickupAddress"`
	PickupCity     string    `json:"pickupCity"`
	PickupZoneID   string    `json:"pickupZoneId"`
	DropoffAddress string    `json:"dropoffAddress"`
	DropoffCity    string    `json:"dropoffCity"`
	DropoffZoneID  string    `json:"dropoffZoneId"`
	WeightKG       float64   `json:"weightKg"`
	SizeCategory   string    `json:"sizeCategory"`
	CreatedAt      time.Time `json:"createdAt"`
}

// UnassignedFilter defines filters for GET /dispatch/unassigned.
type UnassignedFilter struct {
	ZoneID   *string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
	Sort     string // "createdAt:asc" or "createdAt:desc"
}

// UnassignedPage is the paginated result of unassigned parcels.
type UnassignedPage struct {
	Items      []UnassignedParcel
	Page       int
	PageSize   int
	TotalItems int
}

// AssignInput is the payload for assigning a parcel to a courier (API 5.2).
type AssignInput struct {
	ParcelID    string
	CourierID   string
	Reason      string
	ActorUserID string
}

// AssignResult is the returned data for PUT /dispatch/parcels/{id}/assignment.
type AssignResult struct {
	ParcelID          string    `json:"parcelId"`
	TrackingNumber    string    `json:"trackingNumber"`
	CourierID         string    `json:"courierId"`
	PreviousCourierID *string   `json:"previousCourierId"`
	AssignedBy        string    `json:"assignedBy"`
	AssignedAt        time.Time `json:"assignedAt"`
}

// CourierWorkload represents each courier's workload in API 5.3.
type CourierWorkload struct {
	CourierID        string         `json:"courierId"`
	FullName         string         `json:"fullName"`
	IsAvailable      bool           `json:"isAvailable"`
	ActiveDeliveries int            `json:"activeDeliveries"`
	ByStatus         map[string]int `json:"byStatus"`
}

// AdminParcelItem represents one parcel in the admin parcel list (API 5.4).
type AdminParcelItem struct {
	ID             string    `json:"id"`
	TrackingNumber string    `json:"trackingNumber"`
	Status         string    `json:"status"`
	PickupCity     string    `json:"pickupCity"`
	DropoffCity    string    `json:"dropoffCity"`
	CourierID      *string   `json:"courierId"`
	MerchantID     *string   `json:"merchantId"`
	CustomerID     *string   `json:"customerId"`
	Total          float64   `json:"total"`
	CreatedAt      time.Time `json:"createdAt"`
}

// AdminParcelFilter defines query parameters for GET /admin/parcels.
type AdminParcelFilter struct {
	Status         *string
	CourierID      *string
	MerchantID     *string
	CustomerID     *string
	TrackingNumber *string
	From           *time.Time
	To             *time.Time
	Page           int
	PageSize       int
	Sort           string
}

// AdminParcelPage is the paginated admin parcel search result.
type AdminParcelPage struct {
	Items      []AdminParcelItem
	Page       int
	PageSize   int
	TotalItems int
}
