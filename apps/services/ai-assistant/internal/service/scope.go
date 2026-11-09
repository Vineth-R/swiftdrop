package service

import "strings"

// ScopeRefusal is the fixed response for a clearly out-of-scope question.
const ScopeRefusal = "I'm the SwiftDrop Delivery Assistant. I specialize in SwiftDrop services, parcel tracking, delivery pricing and platform-related questions. Is there anything about SwiftDrop I can help you with?"

// swiftdropSignals are words that indicate a SwiftDrop-related request. Their
// presence always permits the request to reach the model, even when it also
// contains a general technical concept.
var swiftdropSignals = []string{
	"swiftdrop", "parcel", "delivery", "deliver", "courier", "track", "tracking",
	"shipment", "shipping", "price", "pricing", "fee", "quote", "zone", "district",
	"pickup", "pick up", "dropoff", "drop off", "drop-off", "merchant", "dispatch",
	"receipt", "weight", "kilogram", " kg", "size", "small", "medium", "large",
	"oversized", "created", "picked_up", "in_transit", "out_for_delivery",
	"delivered", "failed", "colombo", "kandy", "galle", "jaffna", "negombo",
	"lkr", "rupee", "sdk", "sdn", "history", "status", "sender", "recipient",
	"address", "cancel", "refund", "support", "login", "sign in", "sign-in",
	"account", "order",
}

// unrelatedSignals are strong markers of topics outside the assistant's scope.
// The check is deliberately conservative: a SwiftDrop signal always wins, so a
// general technical concept used to explain SwiftDrop is not refused.
var unrelatedSignals = []string{
	"quantum", "politics", "political", "president", "election", "parliament",
	"football", "cricket score", "world cup", "movie", "film", "song", "lyrics",
	"recipe", "poem", "homework", "capital of", "weather forecast", "stock market",
	"bitcoin", "cryptocurrency", "crypto price", "leetcode", "write an essay",
	"translate this", "medical advice", "symptoms", "doctor", "legal advice",
	"investment advice", "dating advice", "horoscope", "who won the",
	"javascript tutorial", "python tutorial", "golang tutorial", "rust tutorial",
	"sql tutorial", "explain quantum", "tell me a joke", "write code for",
}

// OutOfScope reports whether a message is clearly unrelated to SwiftDrop and
// must be refused without calling the provider or any tool. This is a
// defence-in-depth guard; the system instruction remains authoritative.
func OutOfScope(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	for _, signal := range swiftdropSignals {
		if strings.Contains(lower, signal) {
			return false
		}
	}
	for _, signal := range unrelatedSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}
