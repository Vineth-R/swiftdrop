package parcels

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type Store interface {
	ReplayCustomer(context.Context, string, string) (repository.Parcel, error)
	ReplayMerchant(context.Context, string, string) (repository.Parcel, error)
	Create(context.Context, repository.CreateParcelParams, string) (repository.Parcel, error)
	Owned(context.Context, string, string, bool) (repository.Parcel, error)
	List(context.Context, string, Filter) ([]repository.Parcel, int, error)
	History(context.Context, string) ([]repository.StatusHistory, error)
}
type Quoter interface {
	Quote(context.Context, CreateInput) (Fee, error)
}
type keyedStore interface {
	WithKey(context.Context, string, func(Store) (Detail, error)) (Detail, error)
}
type Service struct {
	store    Store
	pricing  Quoter
	couriers CourierReader
}

func NewService(store Store, pricing Quoter) *Service {
	return &Service{store: store, pricing: pricing}
}
func (s *Service) WithCourierReader(reader CourierReader) *Service { s.couriers = reader; return s }

func (s *Service) CreateCustomer(ctx context.Context, userID, key string, in CreateInput) (Detail, error) {
	userID = strings.ToLower(userID)
	key = strings.ToLower(key)
	in = canonicalInput(in)
	if !validation.UUID(userID) {
		return Detail{}, invalid("userId", "must be a UUID")
	}
	if err := in.Validate(true); err != nil {
		return Detail{}, err
	}
	if key != "" && !validation.UUID(key) {
		return Detail{}, invalid("Idempotency-Key", "must be a UUID")
	}
	if s.store == nil {
		return Detail{}, ErrUnavailable
	}
	if key != "" {
		if locked, ok := s.store.(keyedStore); ok {
			return locked.WithKey(ctx, "customer:"+userID+":"+key, func(store Store) (Detail, error) {
				copy := *s
				copy.store = store
				return copy.createCustomer(ctx, userID, key, in)
			})
		}
	}
	return s.createCustomer(ctx, userID, key, in)
}
func (s *Service) createCustomer(ctx context.Context, userID, key string, in CreateInput) (Detail, error) {
	if key != "" {
		p, err := s.store.ReplayCustomer(ctx, userID, key)
		if err == nil {
			return replay(p, in)
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return Detail{}, err
		}
	}
	return s.create(ctx, &userID, nil, nil, key, userID, "CUSTOMER", in)
}
func (s *Service) CreateMerchant(ctx context.Context, in MerchantInput) (Detail, error) {
	in.MerchantID = strings.ToLower(in.MerchantID)
	in.CreatedByUserID = strings.ToLower(in.CreatedByUserID)
	in.Pickup.ZoneID = strings.ToLower(in.Pickup.ZoneID)
	in.Recipient.ZoneID = strings.ToLower(in.Recipient.ZoneID)
	if !validation.UUID(in.MerchantID) {
		return Detail{}, invalid("merchantId", "must be a UUID")
	}
	if strings.ContainsRune(in.ExternalOrderID, '\x00') {
		return Detail{}, invalid("externalOrderId", "must not contain NUL characters")
	}
	if strings.TrimSpace(in.ExternalOrderID) == "" || validation.RuneLength(in.ExternalOrderID) > 100 {
		return Detail{}, invalid("externalOrderId", "must contain 1 to 100 characters")
	}
	if strings.ContainsRune(in.IdempotencyKey, '\x00') {
		return Detail{}, invalid("idempotencyKey", "must not contain NUL characters")
	}
	if validation.RuneLength(in.IdempotencyKey) > 100 {
		return Detail{}, invalid("idempotencyKey", "must be at most 100 characters")
	}
	request := CreateInput{Sender: in.Pickup, Recipient: in.Recipient, WeightKG: in.WeightKG, SizeCategory: in.SizeCategory, Description: in.Description}
	if err := request.Validate(false); err != nil {
		return Detail{}, err
	}
	if !validation.UUID(in.CreatedByUserID) {
		return Detail{}, invalid("createdByUserId", "must be the merchant owner user UUID")
	}
	if s.store == nil {
		return Detail{}, ErrUnavailable
	}
	if locked, ok := s.store.(keyedStore); ok {
		return locked.WithKey(ctx, "merchant:"+in.MerchantID+":"+in.ExternalOrderID, func(store Store) (Detail, error) {
			copy := *s
			copy.store = store
			return copy.createMerchant(ctx, in, request)
		})
	}
	return s.createMerchant(ctx, in, request)
}
func (s *Service) createMerchant(ctx context.Context, in MerchantInput, request CreateInput) (Detail, error) {
	p, err := s.store.ReplayMerchant(ctx, in.MerchantID, in.ExternalOrderID)
	if err == nil {
		if p.CreatedByUserID != in.CreatedByUserID {
			return Detail{}, ErrConflict
		}
		return replay(p, request)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return Detail{}, err
	}
	return s.create(ctx, nil, &in.MerchantID, &in.ExternalOrderID, in.IdempotencyKey, in.CreatedByUserID, "MERCHANT", request)
}
func (s *Service) create(ctx context.Context, customer, merchant, external *string, key, actor, role string, in CreateInput) (Detail, error) {
	if s.pricing == nil {
		return Detail{}, ErrUnavailable
	}
	fee, err := s.pricing.Quote(ctx, in)
	if err != nil {
		return Detail{}, err
	}
	if fee.Currency != "LKR" || fee.BaseFee < 0 || fee.WeightCharge < 0 || fee.SizeSurcharge < 0 || fee.Total < 0 || fee.Total != fee.BaseFee+fee.WeightCharge+fee.SizeSurcharge {
		return Detail{}, ErrUnavailable
	}
	if customer != nil && (in.ExpectedFee == nil || *in.ExpectedFee != fee.Total) {
		return Detail{}, &PriceChanged{CurrentFee: fee}
	}
	params, err := createParams(customer, merchant, external, key, actor, in, fee)
	if err != nil {
		return Detail{}, err
	}
	for attempt := 0; attempt < 5; attempt++ {
		params.TrackingNumber, err = trackingNumber(customer != nil)
		if err != nil {
			return Detail{}, err
		}
		p, err := s.store.Create(ctx, params, role)
		if errors.Is(err, repository.ErrDuplicateTrackingNumber) {
			continue
		}
		if errors.Is(err, repository.ErrDuplicateIdempotencyKey) && customer != nil && key != "" {
			p, err = s.store.ReplayCustomer(ctx, *customer, key)
			if err != nil {
				return Detail{}, err
			}
			return replay(p, in)
		}
		if errors.Is(err, repository.ErrDuplicateMerchantOrder) && merchant != nil && external != nil {
			p, err = s.store.ReplayMerchant(ctx, *merchant, *external)
			if err != nil {
				return Detail{}, err
			}
			return replay(p, in)
		}
		if err != nil {
			return Detail{}, err
		}
		return detail(p)
	}
	return Detail{}, errors.New("tracking number collision retry limit reached")
}
func replay(p repository.Parcel, in CreateInput) (Detail, error) {
	in = canonicalInput(in)
	d, err := detail(p)
	if err != nil {
		return Detail{}, err
	}
	sameDescription := d.Description == nil && in.Description == nil || d.Description != nil && in.Description != nil && *d.Description == *in.Description
	if d.Sender != in.Sender || d.Recipient != in.Recipient || d.WeightKG != in.WeightKG || d.SizeCategory != in.SizeCategory || !sameDescription {
		return Detail{}, ErrConflict
	}
	return d, nil
}
func (s *Service) Detail(ctx context.Context, id, user string, staff bool) (Detail, error) {
	p, err := s.authorized(ctx, id, user, staff)
	if err != nil {
		return Detail{}, err
	}
	d, err := detail(p)
	if err != nil {
		return Detail{}, err
	}
	if p.CurrentCourierID != nil {
		if s.couriers == nil {
			return Detail{}, ErrUnavailable
		}
		courier, err := s.couriers.Lookup(ctx, *p.CurrentCourierID)
		if err != nil {
			return Detail{}, err
		}
		if staff {
			courier.ID = p.CurrentCourierID
		}
		d.Courier = courier
	}
	return d, nil
}
func (s *Service) ReceiptData(ctx context.Context, id, user string, staff bool) (Detail, error) {
	p, err := s.authorized(ctx, id, user, staff)
	if err != nil {
		return Detail{}, err
	}
	return detail(p)
}
func (s *Service) authorized(ctx context.Context, id, user string, staff bool) (repository.Parcel, error) {
	id = strings.ToLower(id)
	user = strings.ToLower(user)
	if !validation.UUID(id) {
		return repository.Parcel{}, invalid("parcelId", "must be a UUID")
	}
	if s.store == nil {
		return repository.Parcel{}, ErrUnavailable
	}
	p, err := s.store.Owned(ctx, id, user, staff)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.Parcel{}, ErrNotFound
	}
	if err != nil {
		return repository.Parcel{}, err
	}
	return p, nil
}
func canonicalInput(in CreateInput) CreateInput {
	in.Sender.ZoneID = strings.ToLower(in.Sender.ZoneID)
	in.Recipient.ZoneID = strings.ToLower(in.Recipient.ZoneID)
	return in
}
func (s *Service) History(ctx context.Context, id, user string, staff bool) ([]HistoryItem, error) {
	if _, err := s.authorized(ctx, id, user, staff); err != nil {
		return nil, err
	}
	rows, err := s.store.History(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryItem, 0, len(rows))
	for _, r := range rows {
		var previous *string
		if r.PreviousStatus != nil {
			v := string(*r.PreviousStatus)
			previous = &v
		}
		out = append(out, HistoryItem{previous, string(r.NewStatus), r.Note, r.FailureReason, r.ActorRole, r.CreatedAt})
	}
	return out, nil
}
func (s *Service) Summary(ctx context.Context, id string) (Summary, error) {
	if !validation.UUID(id) {
		return Summary{}, invalid("parcelId", "must be a UUID")
	}
	if s.store == nil {
		return Summary{}, ErrUnavailable
	}
	p, err := s.store.Owned(ctx, id, "", true)
	if errors.Is(err, repository.ErrNotFound) {
		return Summary{}, ErrNotFound
	}
	if err != nil {
		return Summary{}, err
	}
	return Summary{p.ID, p.TrackingNumber, string(p.Status), p.CustomerUserID, p.MerchantID, p.ExternalOrderID, p.RecipientName}, nil
}
func (s *Service) List(ctx context.Context, user string, f Filter) ([]ListItem, int, error) {
	if !validation.UUID(user) {
		return nil, 0, invalid("userId", "must be a UUID")
	}
	if f.Page < 1 || f.Page > 100000 || f.PageSize < 1 || f.PageSize > 100 {
		return nil, 0, invalid("pagination", "page must be 1 to 100000 and pageSize 1 to 100")
	}
	if f.Sort == "" {
		f.Sort = "createdAt:desc"
	}
	if f.Sort != "createdAt:asc" && f.Sort != "createdAt:desc" {
		return nil, 0, invalid("sort", "must be createdAt:asc or createdAt:desc")
	}
	if f.Status != "" && !validStatus(f.Status) {
		return nil, 0, invalid("status", "unsupported status")
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return nil, 0, invalid("to", "must not precede from")
	}
	if s.store == nil {
		return nil, 0, ErrUnavailable
	}
	rows, total, err := s.store.List(ctx, user, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ListItem, 0, len(rows))
	for _, p := range rows {
		amount, err := numericDecimal(p.PriceAmount)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, ListItem{p.ID, p.TrackingNumber, string(p.Status), p.RecipientName, p.DropoffCity, amount, p.Currency, p.CreatedAt})
	}
	return out, total, nil
}
func uuid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
func trackingNumber(customer bool) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, 8)
	for i := range out {
		for {
			var b [1]byte
			if _, err := rand.Read(b[:]); err != nil {
				return "", err
			}
			if int(b[0]) < 256-(256%len(alphabet)) {
				out[i] = alphabet[int(b[0])%len(alphabet)]
				break
			}
		}
	}
	prefix := "SDN"
	if customer {
		prefix = "SDK"
	}
	return prefix + string(out), nil
}
