package orders

import (
	"context"
	"regexp"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{8,14}$`)

var sizeCategories = []string{"SMALL", "MEDIUM", "LARGE", "OVERSIZED"}

// ZoneLookup resolves the merchant's default pickup zone name. Pricing owns the
// zone data; the merchant service never reads the pricing schema.
type ZoneLookup interface {
	ZoneName(ctx context.Context, zoneID string) (string, error)
}

func normalizeContact(in Contact) Contact {
	return Contact{
		Name:    strings.TrimSpace(in.Name),
		Phone:   strings.TrimSpace(in.Phone),
		Address: strings.TrimSpace(in.Address),
		City:    strings.TrimSpace(in.City),
		ZoneID:  strings.ToLower(strings.TrimSpace(in.ZoneID)),
	}
}

func (s *Service) normalize(ctx context.Context, merchant MerchantProfile, idempotencyKey string, in CreateInput) (Normalized, error) {
	issues := validation.New()

	externalOrderID := strings.TrimSpace(in.ExternalOrderID)
	issues.Check("externalOrderId", !strings.ContainsRune(externalOrderID, '\x00'), "must not contain NUL characters")
	issues.Required("externalOrderId", externalOrderID)
	issues.MaxLength("externalOrderId", externalOrderID, 100, "must be at most 100 characters")

	key := strings.TrimSpace(idempotencyKey)
	issues.Check("Idempotency-Key", !strings.ContainsRune(key, '\x00'), "must not contain NUL characters")
	issues.MaxLength("Idempotency-Key", key, 100, "must be at most 100 characters")

	pickup := Contact{}
	if in.Pickup == nil {
		if s.zones == nil {
			return Normalized{}, ErrZoneUnavailable
		}
		city, err := s.zones.ZoneName(ctx, merchant.PickupZoneID)
		if err != nil {
			return Normalized{}, ErrZoneUnavailable
		}
		pickup = normalizeContact(Contact{
			Name:    merchant.BusinessName,
			Phone:   merchant.ContactPhone,
			Address: merchant.PickupAddress,
			City:    city,
			ZoneID:  merchant.PickupZoneID,
		})
	} else {
		pickup = normalizeContact(*in.Pickup)
	}
	recipient := normalizeContact(in.Recipient)

	validateContact(issues, "pickup", pickup)
	validateContact(issues, "recipient", recipient)

	issues.Check("weightKg", in.WeightKG > 0 && in.WeightKG <= 999999, "must be positive and at most 9999.99")
	issues.Enum("sizeCategory", strings.TrimSpace(in.SizeCategory), "must be SMALL, MEDIUM, LARGE or OVERSIZED", sizeCategories...)

	description := in.Description
	if description != nil {
		trimmed := strings.TrimSpace(*description)
		if trimmed == "" {
			description = nil
		} else {
			description = &trimmed
			issues.Check("description", !strings.ContainsRune(trimmed, '\x00'), "must not contain NUL characters")
			issues.MaxLength("description", trimmed, 255, "must be at most 255 characters")
		}
	}

	if !issues.Empty() {
		return Normalized{}, &Invalid{Issues: issues.Issues()}
	}

	return Normalized{
		ExternalOrderID: externalOrderID,
		IdempotencyKey:  key,
		Pickup:          pickup,
		Recipient:       recipient,
		WeightKG:        in.WeightKG,
		SizeCategory:    strings.TrimSpace(in.SizeCategory),
		Description:     description,
	}, nil
}

func validateContact(issues *validation.Collector, prefix string, contact Contact) {
	issues.Check(prefix+".name", !strings.ContainsRune(contact.Name, '\x00'), "must not contain NUL characters")
	issues.Required(prefix+".name", contact.Name)
	issues.MaxLength(prefix+".name", contact.Name, 150, "must be at most 150 characters")
	issues.Check(prefix+".phone", phonePattern.MatchString(contact.Phone), "must be a valid phone number")
	issues.Check(prefix+".address", !strings.ContainsRune(contact.Address, '\x00'), "must not contain NUL characters")
	issues.Required(prefix+".address", contact.Address)
	issues.MaxLength(prefix+".address", contact.Address, 255, "must be at most 255 characters")
	issues.Required(prefix+".city", contact.City)
	issues.MaxLength(prefix+".city", contact.City, 100, "must be at most 100 characters")
	issues.UUID(prefix+".zoneId", contact.ZoneID)
}
