package orders

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	testMerchantID = "11111111-1111-4111-8111-111111111111"
	testOwnerID    = "22222222-2222-4222-8222-222222222222"
	testZoneID     = "33333333-3333-4333-8333-333333333333"
	testRecipient  = "44444444-4444-4444-8444-444444444444"
	testParcelID   = "55555555-5555-4555-8555-555555555555"
)

func testMerchant() MerchantProfile {
	return MerchantProfile{
		ID: testMerchantID, OwnerUserID: testOwnerID, BusinessName: "ShopZone",
		ContactPhone: "+94112223344", PickupAddress: "88 Galle Road",
		PickupZoneID: testZoneID,
	}
}

func testInput() CreateInput {
	return CreateInput{
		ExternalOrderID: "SZ-100245",
		Recipient: Contact{
			Name: "Amali Fernando", Phone: "+94712345678", Address: "45 Lake Drive",
			City: "Kandy", ZoneID: testRecipient,
		},
		WeightKG:     150,
		SizeCategory: "SMALL",
		Description:  ptr("Phone case"),
	}
}

func ptr(value string) *string { return &value }

func testParcelResult() ParcelResult {
	return ParcelResult{
		ParcelID: testParcelID, TrackingNumber: "SDN4T8W2LC6", Status: "CREATED",
		Fee: Fee{Currency: "LKR", BaseFee: 45000, WeightCharge: 10000, SizeSurcharge: 0, Total: 55000},
	}
}

type fakeStore struct {
	byExternal      map[string]Order
	byKey           map[string]Order
	reservations    map[string]*OrderReservation
	reservationKeys map[string]string
	createCalls     int
	createErr       error
	conflict        bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byExternal: map[string]Order{}, byKey: map[string]Order{},
		reservations: map[string]*OrderReservation{}, reservationKeys: map[string]string{},
	}
}

func (s *fakeStore) OrderByExternalOrderID(_ context.Context, merchantID, externalOrderID string) (Order, error) {
	if order, ok := s.byExternal[merchantID+"|"+externalOrderID]; ok {
		return order, nil
	}
	return Order{}, ErrNotFound
}

func (s *fakeStore) OrderByIdempotencyKey(_ context.Context, merchantID, key string) (Order, error) {
	if order, ok := s.byKey[merchantID+"|"+key]; ok {
		return order, nil
	}
	return Order{}, ErrNotFound
}

func (s *fakeStore) OrderByID(_ context.Context, merchantID, orderID string) (Order, error) {
	for _, order := range s.byExternal {
		if order.MerchantID == merchantID && order.ID == orderID {
			return order, nil
		}
	}
	return Order{}, ErrNotFound
}

func (s *fakeStore) CreateOrder(_ context.Context, params CreateParams) (Order, error) {
	s.createCalls++
	if s.createErr != nil {
		return Order{}, s.createErr
	}
	key := params.MerchantID + "|" + params.ExternalOrderID
	conflict := s.conflict
	s.conflict = false
	if _, exists := s.byExternal[key]; exists {
		return Order{}, ErrDuplicateOrder
	}
	order := Order{
		ID: "order-1", MerchantID: params.MerchantID, ExternalOrderID: params.ExternalOrderID,
		IdempotencyKey: params.IdempotencyKey, RequestHash: params.RequestHash,
		ParcelID: params.ParcelID, TrackingNumber: params.TrackingNumber,
		Pickup: params.Pickup, Recipient: params.Recipient, WeightKG: params.WeightKG,
		SizeCategory: params.SizeCategory, Description: params.Description, Fee: params.Fee,
		Status: params.Status, LastStatusAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.byExternal[key] = order
	if params.IdempotencyKey != nil {
		s.byKey[params.MerchantID+"|"+*params.IdempotencyKey] = order
	}
	if conflict {
		return Order{}, ErrDuplicateOrder
	}
	if params.ReservationID != "" {
		if reservation, ok := s.reservations[params.ReservationID]; ok {
			reservation.Status = OrderReservationCompleted
			reservation.OrderID = &order.ID
		}
	}
	return order, nil
}

func (s *fakeStore) WithIdempotencyLock(_ context.Context, _, _ string, fn func(context.Context) error) error {
	return fn(context.Background())
}

func (s *fakeStore) ReserveOrderAttempt(_ context.Context, merchantID, key, hash string) (OrderReservation, error) {
	lookup := merchantID + "|" + key
	if id, ok := s.reservationKeys[lookup]; ok {
		reservation := s.reservations[id]
		if reservation.RequestHash != hash {
			return OrderReservation{}, ErrDuplicateOrder
		}
		if reservation.Status != OrderReservationCompleted {
			reservation.Status = OrderReservationPending
		}
		return *reservation, nil
	}
	reservation := &OrderReservation{
		ID: "res-" + key, MerchantID: merchantID, IdempotencyKey: key,
		RequestHash: hash, Status: OrderReservationPending,
	}
	s.reservations[reservation.ID] = reservation
	s.reservationKeys[lookup] = reservation.ID
	return *reservation, nil
}

func (s *fakeStore) CompleteOrderAttempt(_ context.Context, reservationID, orderID string) error {
	if reservation, ok := s.reservations[reservationID]; ok {
		reservation.Status = OrderReservationCompleted
		reservation.OrderID = &orderID
	}
	return nil
}

func (s *fakeStore) FailOrderAttempt(_ context.Context, reservationID, _ string) error {
	if reservation, ok := s.reservations[reservationID]; ok && reservation.Status != OrderReservationCompleted {
		reservation.Status = OrderReservationFailed
	}
	return nil
}

func (s *fakeStore) OrderDetail(context.Context, string, string) (Detail, error) {
	return Detail{}, ErrNotFound
}

func (s *fakeStore) ListOrders(context.Context, ListFilter) ([]Order, int, error) {
	return nil, 0, nil
}

type fakeParcel struct {
	calls  []ParcelInput
	result ParcelResult
	err    error
}

func (p *fakeParcel) CreateMerchant(_ context.Context, input ParcelInput) (ParcelResult, error) {
	p.calls = append(p.calls, input)
	if p.err != nil {
		return ParcelResult{}, p.err
	}
	return p.result, nil
}

type fakeZones struct {
	name string
	err  error
}

func (z fakeZones) ZoneName(context.Context, string) (string, error) { return z.name, z.err }

func TestCreateUsesDefaultPickupAndPersistsSnapshot(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, fakeZones{name: "Colombo"})

	result, err := service.Create(context.Background(), testMerchant(), "", testInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !result.Created || result.Order.ID == "" {
		t.Fatalf("Create() = %+v, want a created order", result)
	}
	if len(parcel.calls) != 1 {
		t.Fatalf("parcel calls = %d, want 1", len(parcel.calls))
	}
	input := parcel.calls[0]
	if input.MerchantID != testMerchantID || input.CreatedByUserID != testOwnerID {
		t.Fatalf("parcel identity = %q/%q", input.MerchantID, input.CreatedByUserID)
	}
	if input.Pickup.Name != "ShopZone" || input.Pickup.Phone != "+94112223344" ||
		input.Pickup.Address != "88 Galle Road" || input.Pickup.City != "Colombo" || input.Pickup.ZoneID != testZoneID {
		t.Fatalf("default pickup = %+v", input.Pickup)
	}
	if result.Order.TrackingNumber != "SDN4T8W2LC6" || result.Order.Fee.Total != 55000 {
		t.Fatalf("snapshot = %+v", result.Order)
	}
	if result.Order.RequestHash != Hash(mustNormalize(t, service, "", testInput())) {
		t.Fatalf("request hash was not stable")
	}
	if store.createCalls != 1 {
		t.Fatalf("store create calls = %d, want 1", store.createCalls)
	}
}

func TestExplicitPickupSkipsZoneLookup(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, nil)
	input := testInput()
	input.Pickup = &Contact{Name: "Warehouse", Phone: "+94112223344", Address: "1 Depot Road", City: "Colombo", ZoneID: testZoneID}

	if _, err := service.Create(context.Background(), testMerchant(), "", input); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if parcel.calls[0].Pickup.Name != "Warehouse" {
		t.Fatalf("pickup = %+v", parcel.calls[0].Pickup)
	}
}

func TestCreateReplaysSamePayloadWithoutNewParcel(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, fakeZones{name: "Colombo"})

	first, err := service.Create(context.Background(), testMerchant(), "key-1", testInput())
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	second, err := service.Create(context.Background(), testMerchant(), "key-1", testInput())
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	if second.Created || second.Order.ID != first.Order.ID {
		t.Fatalf("replay = %+v, want original order", second)
	}
	if len(parcel.calls) != 1 {
		t.Fatalf("parcel calls = %d, want 1", len(parcel.calls))
	}
}

func TestCreateRejectsDifferentPayloadForSameKey(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, fakeZones{name: "Colombo"})

	if _, err := service.Create(context.Background(), testMerchant(), "key-1", testInput()); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	different := testInput()
	different.Recipient.Name = "Someone Else"
	_, err := service.Create(context.Background(), testMerchant(), "key-1", different)
	if !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf("Create() error = %v, want ErrDuplicateOrder", err)
	}
	if len(parcel.calls) != 1 {
		t.Fatalf("parcel calls = %d, want 1", len(parcel.calls))
	}
}

func TestCreateRejectsDifferentExternalOrderForSameKey(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, fakeZones{name: "Colombo"})

	if _, err := service.Create(context.Background(), testMerchant(), "key-1", testInput()); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	different := testInput()
	different.ExternalOrderID = "SZ-OTHER"
	_, err := service.Create(context.Background(), testMerchant(), "key-1", different)
	if !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf("Create() error = %v, want ErrDuplicateOrder", err)
	}
}

func TestCreateReplaysConcurrentWinner(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, fakeZones{name: "Colombo"})
	// The insert loses a race: another transaction commits the same order just
	// before this one, so the service must re-read and replay it.
	store.conflict = true
	result, err := service.Create(context.Background(), testMerchant(), "", testInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Created || result.Order.ID == "" {
		t.Fatalf("result = %+v, want the committed winner", result)
	}
	if len(parcel.calls) != 1 {
		t.Fatalf("parcel calls = %d, want 1", len(parcel.calls))
	}
}

func TestCreateReturnsParcelFailures(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{err: &UpstreamInvalid{Code: "ROUTE_NOT_SERVICEABLE"}}
	service := New(store, parcel, fakeZones{name: "Colombo"})

	_, err := service.Create(context.Background(), testMerchant(), "", testInput())
	var upstream *UpstreamInvalid
	if !errors.As(err, &upstream) || upstream.Code != "ROUTE_NOT_SERVICEABLE" {
		t.Fatalf("Create() error = %v, want upstream ROUTE_NOT_SERVICEABLE", err)
	}
	if store.createCalls != 0 {
		t.Fatalf("store create calls = %d, want 0", store.createCalls)
	}
}

func TestCreateRejectsInvalidParcelResult(t *testing.T) {
	store := newFakeStore()
	result := testParcelResult()
	result.Fee.Total = 1
	service := New(store, &fakeParcel{result: result}, fakeZones{name: "Colombo"})

	if _, err := service.Create(context.Background(), testMerchant(), "", testInput()); !errors.Is(err, ErrParcelUnavailable) {
		t.Fatalf("Create() error = %v, want ErrParcelUnavailable", err)
	}
}

func TestCreateReturnsZoneFailure(t *testing.T) {
	service := New(newFakeStore(), &fakeParcel{result: testParcelResult()}, fakeZones{err: errors.New("down")})
	if _, err := service.Create(context.Background(), testMerchant(), "", testInput()); !errors.Is(err, ErrZoneUnavailable) {
		t.Fatalf("Create() error = %v, want ErrZoneUnavailable", err)
	}
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateInput)
		field  string
	}{
		{"missing external order", func(in *CreateInput) { in.ExternalOrderID = "" }, "externalOrderId"},
		{"long external order", func(in *CreateInput) { in.ExternalOrderID = strings.Repeat("a", 101) }, "externalOrderId"},
		{"bad recipient phone", func(in *CreateInput) { in.Recipient.Phone = "0712345678" }, "recipient.phone"},
		{"missing recipient name", func(in *CreateInput) { in.Recipient.Name = " " }, "recipient.name"},
		{"bad recipient zone", func(in *CreateInput) { in.Recipient.ZoneID = "not-a-uuid" }, "recipient.zoneId"},
		{"zero weight", func(in *CreateInput) { in.WeightKG = 0 }, "weightKg"},
		{"large weight", func(in *CreateInput) { in.WeightKG = 1000000 }, "weightKg"},
		{"bad size", func(in *CreateInput) { in.SizeCategory = "HUGE" }, "sizeCategory"},
		{"long description", func(in *CreateInput) { in.Description = ptr(strings.Repeat("x", 256)) }, "description"},
		{"pickup missing city", func(in *CreateInput) {
			in.Pickup = &Contact{Name: "Shop", Phone: "+94112223344", Address: "1 Road", City: "", ZoneID: testZoneID}
		}, "pickup.city"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := testInput()
			test.mutate(&input)
			service := New(newFakeStore(), &fakeParcel{result: testParcelResult()}, fakeZones{name: "Colombo"})
			_, err := service.Create(context.Background(), testMerchant(), "", input)
			var invalid *Invalid
			if !errors.As(err, &invalid) {
				t.Fatalf("Create() error = %v, want Invalid", err)
			}
			found := false
			for _, issue := range invalid.Issues {
				if issue.Field == test.field {
					found = true
				}
			}
			if !found {
				t.Fatalf("issues = %+v, want field %s", invalid.Issues, test.field)
			}
		})
	}
}

func TestCreateRejectsLongIdempotencyKey(t *testing.T) {
	service := New(newFakeStore(), &fakeParcel{result: testParcelResult()}, fakeZones{name: "Colombo"})
	_, err := service.Create(context.Background(), testMerchant(), strings.Repeat("k", 101), testInput())
	var invalid *Invalid
	if !errors.As(err, &invalid) {
		t.Fatalf("Create() error = %v, want Invalid", err)
	}
}

func TestHashIsStableAcrossPropertyOrderAndCase(t *testing.T) {
	service := New(newFakeStore(), nil, fakeZones{name: "Colombo"})
	first := mustNormalize(t, service, "key-1", testInput())

	upper := testInput()
	upper.Recipient.ZoneID = strings.ToUpper(testRecipient)
	upper.Recipient.Name = "  Amali Fernando  "
	second := mustNormalize(t, service, "key-1", upper)

	if Hash(first) != Hash(second) {
		t.Fatalf("hash changed for equivalent requests")
	}
	if Hash(first) == Hash(mustNormalize(t, service, "key-1", func() CreateInput {
		in := testInput()
		in.Recipient.Name = "Other"
		return in
	}())) {
		t.Fatalf("hash did not change for a different payload")
	}
}

func TestDecimalParsing(t *testing.T) {
	var value Decimal
	if err := value.UnmarshalJSON([]byte("1.5")); err != nil || value != 150 {
		t.Fatalf("1.5 = %d, %v", value, err)
	}
	if err := value.UnmarshalJSON([]byte("1.555")); err == nil {
		t.Fatal("three decimal places were accepted")
	}
	if err := value.UnmarshalJSON([]byte("1e2")); err == nil {
		t.Fatal("an exponent was accepted")
	}
	if value.String() != "1.50" {
		t.Fatalf("String() = %q", value.String())
	}
}

func mustNormalize(t *testing.T, service *Service, key string, input CreateInput) Normalized {
	t.Helper()
	normalized, err := service.normalize(context.Background(), testMerchant(), key, input)
	if err != nil {
		t.Fatalf("normalize() error = %v", err)
	}
	return normalized
}

func TestIdempotencyKeyConflictWinsOverExternalOrderReplay(t *testing.T) {
	store := newFakeStore()
	parcel := &fakeParcel{result: testParcelResult()}
	service := New(store, parcel, fakeZones{name: "Colombo"})

	first, err := service.Create(context.Background(), testMerchant(), "key-1", testInput())
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	secondInput := testInput()
	secondInput.ExternalOrderID = "SZ-KEY-TWO"
	if _, err := service.Create(context.Background(), testMerchant(), "key-2", secondInput); err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	// key-2 already names SZ-KEY-TWO; reusing it for SZ-100245 must conflict
	// even though SZ-100245 exists from key-1.
	if _, err := service.Create(context.Background(), testMerchant(), "key-2", testInput()); !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf("Create() error = %v, want ErrDuplicateOrder", err)
	}
	if len(parcel.calls) != 2 {
		t.Fatalf("parcel calls = %d, want 2", len(parcel.calls))
	}
	_ = first
}
