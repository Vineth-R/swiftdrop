//go:build integration && backendintegration

package integration

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// Run only against a disposable local Compose stack. Fixtures use fresh UUIDs;
// cleanup deletes only rows owned by this invocation, never resets a volume.
func TestPricingTrackingBackend(t *testing.T) {
	base := strings.TrimRight(os.Getenv("TEST_BASE_URL"), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "http" {
		t.Fatal("set TEST_BASE_URL to the local HTTP gateway")
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		t.Fatal("backend fixture tests require a local gateway")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("docker", "compose", "config", "--format", "json")
	command.Dir = root
	raw, err := command.Output()
	if err != nil {
		t.Fatal("cannot read local Compose configuration; start Docker first")
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal("cannot parse local Compose configuration")
	}
	secret := config.Services["pricing"].Environment["JWT_ACCESS_SECRET"]
	if secret == "" {
		t.Fatal("configure JWT_ACCESS_SECRET for admin integration fixtures")
	}
	actor := backendUUID(t)
	manager, err := auth.NewTokenManager(secret, 30*time.Minute)
	if err != nil {
		t.Fatal("configure JWT_ACCESS_SECRET for admin integration fixtures")
	}
	token, _, err := manager.Issue(actor, auth.RoleAdmin)
	if err != nil {
		t.Fatal("cannot sign test identity")
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(method, path, body string, authenticated bool) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if authenticated {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("gateway request failed", err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data
	}
	sql := func(statement string) string {
		t.Helper()
		cmd := exec.Command("docker", "compose", "exec", "-T", "postgres", "sh", "-c", `exec psql -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"`)
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(statement)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal("fixture SQL failed; verify migrations and PostgreSQL")
		}
		return strings.TrimSpace(string(output))
	}
	origin, destination, rule := backendUUID(t), backendUUID(t), backendUUID(t)
	bandIDs := []string{}
	parcelIDs := []string{}
	// Register cleanup before inserting, so a partially created fixture is scoped safely.
	t.Cleanup(func() {
		// parcel_status_history is append-only in production. Disable trigger
		// firing for this disposable cleanup session only; the SET expires with
		// the psql session and does not change any persistent database setting.
		cleanup := "SET session_replication_role = replica;\n"
		if len(parcelIDs) > 0 {
			cleanup += fmt.Sprintf("DELETE FROM parcel.parcel_status_history WHERE parcel_id IN (%s);\n", quotedUUIDList(parcelIDs))
			cleanup += fmt.Sprintf("DELETE FROM parcel.parcels WHERE id IN (%s);\n", quotedUUIDList(parcelIDs))
		}
		cleanup += fmt.Sprintf("DELETE FROM pricing.audit_logs WHERE actor_user_id='%s'; DELETE FROM pricing.pricing_rules WHERE id='%s';\n", actor, rule)
		for _, id := range bandIDs {
			cleanup += fmt.Sprintf("DELETE FROM pricing.weight_bands WHERE id='%s';\n", id)
		}
		cleanup += fmt.Sprintf("DELETE FROM pricing.zones WHERE id IN ('%s','%s');\n", origin, destination)
		cleanup += "RESET session_replication_role;"
		sql(cleanup)
	})
	sql(fmt.Sprintf(`INSERT INTO pricing.zones(id,code,name,districts) VALUES
	 ('%s','%s','Fixture origin',ARRAY['Fixture']),('%s','%s','Fixture destination',ARRAY['Fixture']);
	 INSERT INTO pricing.pricing_rules(id,origin_zone_id,destination_zone_id,base_fee) VALUES('%s','%s','%s',450);`,
		origin, "T"+strings.ReplaceAll(origin, "-", "")[:12], destination, "T"+strings.ReplaceAll(destination, "-", "")[:12], rule, origin, destination))
	quoteBody := func(weight, size string) string {
		return fmt.Sprintf(`{"originZoneId":"%s","destinationZoneId":"%s","weightKg":%s,"sizeCategory":"%s"}`, origin, destination, weight, size)
	}
	for _, test := range []struct {
		name, weight, size string
		total              float64
	}{
		{name: "seed", weight: "2", size: "SMALL", total: 550},
		{name: "one kg edge", weight: "1", size: "SMALL", total: 450},
		{name: "five kg edge", weight: "5", size: "SMALL", total: 550},
		{name: "next band", weight: "5.01", size: "SMALL", total: 700},
		{name: "ten kg edge", weight: "10", size: "SMALL", total: 700},
		{name: "higher band", weight: "10.01", size: "SMALL", total: 950},
		{name: "medium", weight: "2", size: "MEDIUM", total: 700},
		{name: "large", weight: "2", size: "LARGE", total: 950},
		{name: "oversized", weight: "2", size: "OVERSIZED", total: 1450},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, data := call("POST", "/api/v1/pricing/estimate", quoteBody(test.weight, test.size), false)
			var response struct {
				Data struct {
					Total float64 `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if status != 200 || response.Data.Total != test.total {
				t.Fatalf("quote status %d total %v, body %s", status, response.Data.Total, data)
			}
		})
	}
	t.Run("admin authentication", func(t *testing.T) {
		status, _ := call("GET", "/api/v1/admin/pricing/rules", "", false)
		if status != 401 {
			t.Fatalf("anonymous admin status %d", status)
		}
	})
	t.Run("unknown zone", func(t *testing.T) {
		body := strings.Replace(quoteBody("2", "SMALL"), origin, backendUUID(t), 1)
		status, data := call("POST", "/api/v1/pricing/estimate", body, false)
		if status != 404 || !bytes.Contains(data, []byte("ZONE_NOT_FOUND")) {
			t.Fatal(status, string(data))
		}
	})
	t.Run("unpriced route", func(t *testing.T) {
		body := strings.Replace(quoteBody("2", "SMALL"), destination, origin, 1)
		status, data := call("POST", "/api/v1/pricing/estimate", body, false)
		if status != 422 || !bytes.Contains(data, []byte("ROUTE_NOT_SERVICEABLE")) {
			t.Fatal(status, string(data))
		}
	})
	t.Run("band boundaries and conflicts", func(t *testing.T) {
		for _, body := range []string{`{"minWeightKg":50,"maxWeightKg":51,"charge":200}`, `{"minWeightKg":51,"maxWeightKg":52,"charge":200}`} {
			status, data := call("POST", "/api/v1/admin/pricing/bands", body, true)
			if status != 201 {
				t.Fatal(status, string(data))
			}
			bandIDs = append(bandIDs, backendResponseID(t, data))
		}
		status, data := call("POST", "/api/v1/admin/pricing/bands", `{"minWeightKg":50.5,"maxWeightKg":51.5,"charge":1}`, true)
		if status != 409 || !bytes.Contains(data, []byte("BAND_OVERLAP")) {
			t.Fatal(status, string(data))
		}
		for _, test := range []struct{ weight, code string }{{weight: "50", code: "WEIGHT_NOT_PRICED"}, {weight: "52.01", code: "WEIGHT_NOT_PRICED"}} {
			status, data := call("POST", "/api/v1/pricing/estimate", quoteBody(test.weight, "SMALL"), false)
			if status != 422 || !bytes.Contains(data, []byte(test.code)) {
				t.Fatal(status, string(data))
			}
		}
		status, data = call("POST", "/api/v1/pricing/estimate", quoteBody("51", "SMALL"), false)
		if status != 200 || !bytes.Contains(data, []byte(`"total":650.00`)) {
			t.Fatal(status, string(data))
		}
	})
	trackingNumbers := []string{}
	for _, state := range []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"} {
		id := backendUUID(t)
		parcelIDs = append(parcelIDs, id)
		number := "SDK" + strings.ToUpper(strings.ReplaceAll(id, "-", "")[:8])
		trackingNumbers = append(trackingNumbers, number)
		sql(fmt.Sprintf(`INSERT INTO parcel.parcels(id,tracking_number,customer_user_id,sender_name,sender_phone,pickup_address,pickup_city,pickup_zone_id,
		 recipient_name,recipient_phone,dropoff_address,dropoff_city,dropoff_zone_id,weight_kg,size_category,status,
		 price_amount,price_breakdown,created_by_user_id,delivered_at,delivered_to_name,failed_at,failure_reason)
		 VALUES('%s','%s','%s','private-sender','private-phone','private-address','Fixture origin','%s',
		 'private-recipient','private-phone','private-address','Fixture destination','%s',2,'SMALL','%s',
		 550,'{"base_fee":450,"weight_charge":100,"size_surcharge":0}','%s',
		 CASE WHEN '%s'='DELIVERED' THEN now() END,CASE WHEN '%s'='DELIVERED' THEN 'private-recipient' END,
		 CASE WHEN '%s'='FAILED' THEN now() END,CASE WHEN '%s'='FAILED' THEN 'private-reason' END);
		 INSERT INTO parcel.parcel_status_history(parcel_id,new_status,created_at,note,failure_reason,actor_user_id)
		 VALUES('%s','%s',now(),'private-note',CASE WHEN '%s'='FAILED' THEN 'private-reason' END,'%s');
		 INSERT INTO parcel.parcel_status_history(parcel_id,new_status,created_at)
		 SELECT '%s','CREATED',now()-interval '1 hour' WHERE '%s'<>'CREATED';`,
			id, number, actor, origin, destination, state, actor, state, state, state, state, id, state, state, actor, id, state))
		t.Run("tracking "+state, func(t *testing.T) {
			status, data := call("GET", "/api/v1/tracking/"+number, "", false)
			if status != 200 || bytes.Contains(data, []byte("private-")) || bytes.Contains(data, []byte(actor)) {
				t.Fatal(status, string(data))
			}
			var response struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Data) != 6 {
				t.Fatalf("unexpected public fields: %s", data)
			}
			var timeline []map[string]json.RawMessage
			if err := json.Unmarshal(response.Data["timeline"], &timeline); err != nil {
				t.Fatal(err)
			}
			wantedEvents := 2
			if state == "CREATED" {
				wantedEvents = 1
			}
			if len(timeline) != wantedEvents || len(timeline[0]) != 2 {
				t.Fatalf("unexpected public history: %s", data)
			}
			if wantedEvents == 2 && string(timeline[0]["status"]) != `"CREATED"` {
				t.Fatalf("history is not chronological: %s", data)
			}
			if string(response.Data["status"]) != `"`+state+`"` {
				t.Fatal(string(data))
			}
		})
	}
	t.Run("stored fee remains unchanged", func(t *testing.T) {
		status, data := call("PATCH", "/api/v1/admin/pricing/rules/"+rule, `{"baseFee":500}`, true)
		if status != 200 {
			t.Fatal(status, string(data))
		}
		status, data = call("POST", "/api/v1/pricing/estimate", quoteBody("2", "SMALL"), false)
		if status != 200 || !bytes.Contains(data, []byte(`"total":600.00`)) {
			t.Fatal(status, string(data))
		}
		if sql(fmt.Sprintf("SELECT price_amount::text FROM parcel.parcels WHERE id='%s';", parcelIDs[0])) != "550.00" {
			t.Fatal("stored parcel fee changed")
		}
		if sql(fmt.Sprintf("SELECT count(*)>0 FROM pricing.audit_logs WHERE actor_user_id='%s';", actor)) != "t" {
			t.Fatal("audit record missing")
		}
	})
	t.Run("inactive zones hidden", func(t *testing.T) {
		status, data := call("PATCH", "/api/v1/admin/zones/"+origin, `{"isActive":false}`, true)
		if status != 200 {
			t.Fatal(status, string(data))
		}
		status, data = call("GET", "/api/v1/zones", "", false)
		if status != 200 || bytes.Contains(data, []byte(origin)) {
			t.Fatal(status, string(data))
		}
		status, data = call("POST", "/api/v1/pricing/estimate", quoteBody("2", "SMALL"), false)
		if status != 404 {
			t.Fatal(status, string(data))
		}
	})
	t.Run("tracking unknown", func(t *testing.T) {
		number := "SDN" + strings.ToUpper(strings.ReplaceAll(backendUUID(t), "-", "")[:8])
		status, data := call("GET", "/api/v1/tracking/"+number, "", false)
		if status != 404 || !bytes.Contains(data, []byte("TRACKING_NOT_FOUND")) {
			t.Fatal(status, string(data))
		}
	})
	t.Run("internal edge blocked", func(t *testing.T) {
		status, _ := call("POST", "/internal/pricing/quote", quoteBody("2", "SMALL"), false)
		if status != 404 {
			t.Fatal(status)
		}
	})
	t.Run("tracking rate limit resists spoofing", func(t *testing.T) {
		for index := 0; index < 31; index++ {
			request, err := http.NewRequest("GET", base+"/api/v1/tracking/"+trackingNumbers[0], nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("X-Real-IP", fmt.Sprintf("203.0.113.%d", index))
			request.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", index))
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode == 429 {
				if response.Header.Get("Retry-After") == "" {
					t.Fatal("missing Retry-After")
				}
				return
			}
			if response.StatusCode != 200 {
				t.Fatal(response.StatusCode)
			}
		}
		t.Fatal("tracking rate limit bypassed")
	})
}

func quotedUUIDList(ids []string) string {
	quoted := make([]string, len(ids))
	for index, id := range ids {
		quoted[index] = "'" + id + "'"
	}
	return strings.Join(quoted, ",")
}

func backendUUID(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
func backendResponseID(t *testing.T, data []byte) string {
	t.Helper()
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.ID) != 36 || strings.ContainsAny(response.Data.ID, "'\";") {
		t.Fatal("invalid resource identifier")
	}
	return response.Data.ID
}
