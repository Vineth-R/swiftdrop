package orders

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// hashPayload is a fixed-order structure. Hashing a struct, not a map, keeps the
// digest stable regardless of JSON property order on the wire.
type hashPayload struct {
	ExternalOrderID string  `json:"externalOrderId"`
	Pickup          Contact `json:"pickup"`
	Recipient       Contact `json:"recipient"`
	WeightKG        string  `json:"weightKg"`
	SizeCategory    string  `json:"sizeCategory"`
	Description     *string `json:"description"`
}

// Hash returns the stable SHA-256 hex digest of a normalized request. The
// idempotency key is an identifier, not payload content, so it is excluded.
func Hash(in Normalized) string {
	payload := hashPayload{
		ExternalOrderID: in.ExternalOrderID,
		Pickup:          in.Pickup,
		Recipient:       in.Recipient,
		WeightKG:        in.WeightKG.String(),
		SizeCategory:    in.SizeCategory,
		Description:     in.Description,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
