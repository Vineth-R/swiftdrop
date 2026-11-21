package parcels

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPricingResponseContract(t *testing.T) {
	valid := map[string]any{"currency": "LKR", "baseFee": 0, "weightCharge": 0, "sizeSurcharge": 0, "total": 0}
	encode := func(fields map[string]any) string {
		raw, err := json.Marshal(map[string]any{"data": fields})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	copyFields := func() map[string]any {
		fields := make(map[string]any, len(valid))
		for name, value := range valid {
			fields[name] = value
		}
		return fields
	}
	type quoteCase struct {
		name, body string
		valid      bool
	}
	tests := []quoteCase{
		{"complete zero fee", encode(valid), true},
		{"missing data", `{}`, false},
		{"null data", `{"data":null}`, false},
		{"array data", `{"data":[]}`, false},
		{"LKR only", `{"data":{"currency":"LKR"}}`, false},
		{"empty currency", `{"data":{"currency":"","baseFee":0,"weightCharge":0,"sizeSurcharge":0,"total":0}}`, false},
		{"unsupported currency", `{"data":{"currency":"USD","baseFee":0,"weightCharge":0,"sizeSurcharge":0,"total":0}}`, false},
	}
	for _, field := range []string{"currency", "baseFee", "weightCharge", "sizeSurcharge", "total"} {
		missing := copyFields()
		delete(missing, field)
		tests = append(tests, quoteCase{"missing " + field, encode(missing), false})
		null := copyFields()
		null[field] = nil
		tests = append(tests, quoteCase{"null " + field, encode(null), false})
		type invalidType struct {
			name  string
			value any
		}
		wrongTypes := []invalidType{{"boolean", true}, {"object", map[string]any{}}, {"array", []any{}}}
		if field == "currency" {
			wrongTypes = append(wrongTypes, invalidType{"number", 0})
		} else {
			wrongTypes = append(wrongTypes, invalidType{"string", "0"})
		}
		for _, invalid := range wrongTypes {
			wrong := copyFields()
			wrong[field] = invalid.value
			tests = append(tests, quoteCase{field + " invalid " + invalid.name, encode(wrong), false})
		}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			fee, err := NewPricingClient(server.URL, "pricing-contract-test", server.Client()).Quote(t.Context(), validInput())
			if tc.valid {
				if err != nil || fee != (Fee{Currency: "LKR"}) {
					t.Fatalf("complete zero quote: fee=%+v error=%v", fee, err)
				}
			} else if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("malformed quote accepted: fee=%+v error=%v", fee, err)
			}
		})
	}
}
