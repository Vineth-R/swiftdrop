//go:build access

package access

import "testing"

// ownershipCases are the "you can only touch your own" rules from the team task
// sheet. Each needs two real accounts of the same role and a record owned by
// one of them, so each needs the owning service's create endpoint. They are
// listed here with their exact request and answer, so the owner turns each
// into a real test the day the endpoint exists: create the record as account A,
// repeat Request as account B, expect Want, and delete the Skip.
//
// Keep Want exactly as written. A 404 for another customer's parcel (not 403)
// is deliberate, so a caller cannot learn that the parcel exists.
var ownershipCases = []struct {
	rule    string
	request string
	want    string
	blocked string
}{
	{
		rule:    "a customer cannot read another customer's parcel",
		request: "GET /api/v1/parcels/{parcelId} as customer B, for a parcel customer A created (API 4.3)",
		want:    "404 NOT_FOUND, with no part of the parcel in the body",
		blocked: "the parcel service has no POST /parcels or GET /parcels/{id} yet",
	},
	{
		rule:    "a courier cannot update another courier's parcel",
		request: "POST /api/v1/courier/deliveries/{parcelId}/status as courier B, for a parcel assigned to courier A (API 6.3)",
		want:    "403 FORBIDDEN, and the parcel status is unchanged",
		blocked: "the parcel service has no courier delivery routes or dispatch assignment yet",
	},
	{
		rule:    "a merchant cannot see another merchant's order",
		request: "GET /api/v1/merchant/orders/{orderId} as merchant B, for an order merchant A created (API 8.18)",
		want:    "404 NOT_FOUND, with no part of the order in the body",
		blocked: "the merchant service has no order routes yet and does not verify access tokens",
	},
}

func TestOwnership(t *testing.T) {
	for _, c := range ownershipCases {
		t.Run(c.rule, func(t *testing.T) {
			t.Skipf("not checkable yet: %s.\nRequest: %s\nExpect: %s", c.blocked, c.request, c.want)
		})
	}
}
