package servicehttp

import (
	"reflect"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

func TestValidateProfileRequestPreservesWhitespaceRequiredSemantics(t *testing.T) {
	tests := []struct {
		name    string
		request profileRequest
		want    []validation.Issue
	}{
		{
			name: "whitespace-only values remain accepted",
			request: profileRequest{
				ContactPhone:  "   ",
				PickupAddress: "\t",
				PickupZoneID:  "  ",
			},
			want: nil,
		},
		{
			name: "empty contact phone",
			request: profileRequest{
				PickupAddress: "12 Temple Road",
				PickupZoneID:  "3f1c7a52-0000-4000-8000-000000000001",
			},
			want: []validation.Issue{{Field: "contactPhone", Issue: "is required"}},
		},
		{
			name: "empty pickup address",
			request: profileRequest{
				ContactPhone: "+94771234567",
				PickupZoneID: "3f1c7a52-0000-4000-8000-000000000001",
			},
			want: []validation.Issue{{Field: "pickupAddress", Issue: "is required"}},
		},
		{
			name: "empty pickup zone",
			request: profileRequest{
				ContactPhone:  "+94771234567",
				PickupAddress: "12 Temple Road",
			},
			want: []validation.Issue{{Field: "pickupZoneId", Issue: "is required"}},
		},
		{
			name:    "all empty fields report in stable order",
			request: profileRequest{},
			want: []validation.Issue{
				{Field: "contactPhone", Issue: "is required"},
				{Field: "pickupAddress", Issue: "is required"},
				{Field: "pickupZoneId", Issue: "is required"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validateProfileRequest(test.request); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("validateProfileRequest() = %+v, want %+v", got, test.want)
			}
		})
	}
}
