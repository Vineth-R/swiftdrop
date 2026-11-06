package parcels

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
)

const testUser = "11111111-1111-4111-8111-111111111111"
const testOther = "22222222-2222-4222-8222-222222222222"

type testQuoter struct {
	fee   Fee
	err   error
	calls int
}

func (q *testQuoter) Quote(context.Context, CreateInput) (Fee, error) { q.calls++; return q.fee, q.err }

type memoryStore struct {
	p         repository.Parcel
	exists    bool
	createErr error
	replayErr error
	calls     int
	history   []repository.StatusHistory
}

func (m *memoryStore) ReplayCustomer(context.Context, string, string) (repository.Parcel, error) {
	if m.replayErr != nil {
		return repository.Parcel{}, m.replayErr
	}
	if m.exists {
		return m.p, nil
	}
	return repository.Parcel{}, repository.ErrNotFound
}
func (m *memoryStore) ReplayMerchant(ctx context.Context, a, b string) (repository.Parcel, error) {
	return m.ReplayCustomer(ctx, a, b)
}
func (m *memoryStore) Create(_ context.Context, p repository.CreateParcelParams, _ string) (repository.Parcel, error) {
	m.calls++
	if m.createErr != nil {
		return repository.Parcel{}, m.createErr
	}
	m.p = repository.Parcel{ID: p.ID, TrackingNumber: p.TrackingNumber, CustomerUserID: p.CustomerUserID, MerchantID: p.MerchantID, ExternalOrderID: p.ExternalOrderID, SenderName: p.SenderName, SenderPhone: p.SenderPhone, PickupAddress: p.PickupAddress, PickupCity: p.PickupCity, PickupZoneID: p.PickupZoneID, RecipientName: p.RecipientName, RecipientPhone: p.RecipientPhone, DropoffAddress: p.DropoffAddress, DropoffCity: p.DropoffCity, DropoffZoneID: p.DropoffZoneID, WeightKG: p.WeightKG, SizeCategory: p.SizeCategory, Description: p.Description, Status: repository.ParcelStatusCreated, PriceAmount: p.PriceAmount, Currency: p.Currency, PriceBreakdown: p.PriceBreakdown, CreatedByUserID: p.CreatedByUserID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	m.exists = true
	return m.p, nil
}
func (m *memoryStore) Owned(_ context.Context, id, user string, staff bool) (repository.Parcel, error) {
	if m.exists && m.p.ID == id && (staff || m.p.CustomerUserID != nil && *m.p.CustomerUserID == user) {
		return m.p, nil
	}
	return repository.Parcel{}, repository.ErrNotFound
}
func (m *memoryStore) List(_ context.Context, user string, f Filter) ([]repository.Parcel, int, error) {
	if m.exists {
		return []repository.Parcel{m.p}, 1, nil
	}
	return []repository.Parcel{}, 0, nil
}
func (m *memoryStore) History(context.Context, string) ([]repository.StatusHistory, error) {
	return m.history, nil
}
func validInput() CreateInput {
	expected := Decimal(55000)
	return CreateInput{Sender: Contact{"Sender", "+94771234567", "12 Road", "Colombo", testUser}, Recipient: Contact{"Recipient", "+94712345678", "45 Road", "Kandy", testOther}, WeightKG: 200, SizeCategory: "SMALL", ExpectedFee: &expected}
}
func quote550() *testQuoter { return &testQuoter{fee: Fee{"LKR", 45000, 10000, 0, 55000}} }
func TestCustomerCreateReplayPriceAndOwnership(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	q := quote550()
	s := NewService(store, q)
	in := validInput()
	created, err := s.CreateCustomer(ctx, testUser, testOther, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.Fee.Total != 55000 || !strings.HasPrefix(created.TrackingNumber, "SDK") {
		t.Fatalf("wrong created data: %+v", created)
	}
	q.fee.Total = 90000
	again, err := s.CreateCustomer(ctx, testUser, testOther, in)
	if err != nil || again.ID != created.ID || q.calls != 1 || store.calls != 1 {
		t.Fatalf("replay called dependencies: %v %+v", err, again)
	}
	in.Recipient.Name = "Changed"
	if _, err := s.CreateCustomer(ctx, testUser, testOther, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
	if _, err := s.Detail(ctx, created.ID, testOther, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner read: %v", err)
	}
	if _, err := s.Detail(ctx, created.ID, testOther, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.History(ctx, created.ID, testOther, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner history: %v", err)
	}
	summary, err := s.Summary(ctx, created.ID)
	if err != nil || summary.CustomerUserID == nil || *summary.CustomerUserID != testUser {
		t.Fatalf("summary %+v %v", summary, err)
	}
	rows, total, err := s.List(ctx, testUser, Filter{Page: 1, PageSize: 20})
	if err != nil || len(rows) != 1 || total != 1 || rows[0].Total != 55000 {
		t.Fatalf("list %+v %d %v", rows, total, err)
	}
}
func TestCreateRejectsPriceAndDependencyFailures(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		q    *testQuoter
		want error
	}{{"pricing unreachable", &testQuoter{err: ErrUnavailable}, ErrUnavailable}, {"wrong currency", &testQuoter{fee: Fee{"USD", 45000, 10000, 0, 55000}}, ErrUnavailable}, {"inconsistent total", &testQuoter{fee: Fee{"LKR", 45000, 10000, 0, 55001}}, ErrUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			_, err := NewService(store, tc.q).CreateCustomer(ctx, testUser, "", validInput())
			if !errors.Is(err, tc.want) || store.calls != 0 {
				t.Fatalf("got %v, writes %d", err, store.calls)
			}
		})
	}
	store := &memoryStore{}
	q := quote550()
	q.fee = Fee{"LKR", 50000, 10000, 0, 60000}
	_, err := NewService(store, q).CreateCustomer(ctx, testUser, "", validInput())
	var price *PriceChanged
	if !errors.As(err, &price) || price.CurrentFee.Total != 60000 || store.calls != 0 {
		t.Fatalf("price mismatch %v writes%d", err, store.calls)
	}
	if _, err := NewService(nil, nil).CreateCustomer(ctx, testUser, "", validInput()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := NewService(&memoryStore{}, nil).CreateCustomer(ctx, testUser, "", validInput()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	collision := &memoryStore{createErr: repository.ErrDuplicateTrackingNumber}
	if _, err := NewService(collision, quote550()).CreateCustomer(ctx, testUser, "", validInput()); err == nil || collision.calls != 5 {
		t.Fatalf("collision retries%d %v", collision.calls, err)
	}
}
func TestValidationAndDecimals(t *testing.T) {
	for _, raw := range []string{"null", "\"1\"", "0.001", "1e999999999", "1e-9999999", "99999999999999", "1.2.3"} {
		var d Decimal
		if err := json.Unmarshal([]byte(raw), &d); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"1", "1.20", "120e-2", "-1.25"} {
		var d Decimal
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
	if Decimal(-125).String() != "-1.25" {
		t.Fatal("wrong negative decimal")
	}
	in := validInput()
	in.Sender = Contact{}
	in.Recipient = Contact{}
	in.WeightKG = 0
	in.SizeCategory = "HUGE"
	in.Description = new(string)
	*in.Description = strings.Repeat("x", 256)
	in.ExpectedFee = nil
	var invalid *Invalid
	if err := in.Validate(true); !errors.As(err, &invalid) || len(invalid.Issues) < 10 {
		t.Fatalf("missing validation: %v", err)
	}
	s := NewService(&memoryStore{}, quote550())
	if _, err := s.CreateCustomer(context.Background(), "bad", "", validInput()); err == nil {
		t.Fatal("bad user")
	}
	if _, err := s.CreateCustomer(context.Background(), testUser, "bad", validInput()); err == nil {
		t.Fatal("bad key")
	}
	for _, f := range []Filter{{Page: 0, PageSize: 20}, {Page: 1, PageSize: 101}, {Page: 1, PageSize: 20, Sort: "id;drop table"}, {Page: 1, PageSize: 20, Status: "FAKE"}} {
		if _, _, err := s.List(context.Background(), testUser, f); err == nil {
			t.Errorf("accepted %+v", f)
		}
	}
}
func TestTrackingRandomnessAndReceipt(t *testing.T) {
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`^SDK[A-HJ-NP-Z2-9]{8}$`)
	for i := 0; i < 10000; i++ {
		n, err := trackingNumber(true)
		if err != nil || !pattern.MatchString(n) || seen[n] {
			t.Fatalf("bad tracking %s %v", n, err)
		}
		seen[n] = true
	}
	n, err := trackingNumber(false)
	if err != nil || !strings.HasPrefix(n, "SDN") {
		t.Fatal(n, err)
	}
	store := &memoryStore{}
	p, err := NewService(store, quote550()).CreateCustomer(context.Background(), testUser, "", validInput())
	if err != nil {
		t.Fatal(err)
	}
	p.Sender.Name = "Nimal (test) \\"
	pdf := string(Receipt(p))
	for _, part := range []string{"%PDF-1.4", p.TrackingNumber, "Total fee: 550.00", "Base fee: 450.00", "Weight charge: 100.00", "%%EOF", "/ActualText", "Nimal \\(test\\)"} {
		if !strings.Contains(pdf, part) {
			t.Errorf("receipt missing %q", part)
		}
	}
}

func TestReceiptWrapsAndPaginatesLongFields(t *testing.T) {
	store := &memoryStore{}
	p, err := NewService(store, quote550()).CreateCustomer(context.Background(), testUser, "", validInput())
	if err != nil {
		t.Fatal(err)
	}
	// Words just over half a line force the maximum number of wrapped lines.
	longName := strings.TrimSpace(strings.Repeat(strings.Repeat("W", 21)+" ", 7))
	longCity := strings.TrimSpace(strings.Repeat(strings.Repeat("M", 21)+" ", 4))
	p.Sender.Name = longName
	p.Recipient.Name = longName
	p.Sender.City = longCity
	p.Recipient.City = longCity
	for _, line := range receiptLines("Sender: " + longName) {
		if len([]rune(line)) > 40 {
			t.Fatalf("overlong receipt line %q", line)
		}
	}
	for _, line := range receiptLines(strings.Repeat("X", 150)) {
		if len(line) > 40 {
			t.Fatal("unbroken word was not wrapped")
		}
	}
	delivered := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	p.DeliveredAt = &delivered
	pdf := string(Receipt(p))
	if !strings.Contains(pdf, "/Count 2") || !strings.Contains(pdf, "Delivered:") {
		t.Fatal("long receipt was not paginated or lost delivered date")
	}
}
func TestMerchantAndFailures(t *testing.T) {
	ctx := context.Background()
	in := validInput()
	merchant := MerchantInput{MerchantID: testOther, CreatedByUserID: testUser, ExternalOrderID: "ORDER-1", Pickup: in.Sender, Recipient: in.Recipient, WeightKG: in.WeightKG, SizeCategory: in.SizeCategory}
	store := &memoryStore{}
	s := NewService(store, quote550())
	p, err := s.CreateMerchant(ctx, merchant)
	if err != nil || !strings.HasPrefix(p.TrackingNumber, "SDN") || store.p.CustomerUserID != nil || store.p.CreatedByUserID != testUser {
		t.Fatalf("merchant %+v %v", p, err)
	}
	p2, err := s.CreateMerchant(ctx, merchant)
	if err != nil || p2.ID != p.ID {
		t.Fatalf("merchant replay %+v %v", p2, err)
	}
	merchant.CreatedByUserID = ""
	if _, err := s.CreateMerchant(ctx, merchant); err == nil {
		t.Fatal("missing actor")
	}
	merchant.CreatedByUserID = testUser
	merchant.MerchantID = "bad"
	if _, err := s.CreateMerchant(ctx, merchant); err == nil {
		t.Fatal("bad merchant")
	}
	for _, method := range []func() error{func() error { _, e := s.Detail(ctx, "bad", testUser, false); return e }, func() error { _, e := s.Summary(ctx, "bad"); return e }} {
		if method() == nil {
			t.Fatal("bad UUID accepted")
		}
	}
	if _, err := s.Summary(ctx, testUser); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
