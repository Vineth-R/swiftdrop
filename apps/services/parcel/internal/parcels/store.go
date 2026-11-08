package parcels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Database interface {
	repository.DB
	Begin(context.Context) (pgx.Tx, error)
}
type PostgresStore struct{ db Database }

func NewStore(db Database) *PostgresStore { return &PostgresStore{db} }

// WithKey serializes a single owner's intended submission across instances.
// The lock and all writes share an outer transaction; Create uses a savepoint,
// allowing a tracking collision to be retried without aborting that transaction.
func (s *PostgresStore) WithKey(ctx context.Context, key string, fn func(Store) (Detail, error)) (Detail, error) {
	if s.db == nil {
		return Detail{}, ErrUnavailable
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin idempotent parcel request: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		return Detail{}, fmt.Errorf("lock idempotent parcel request: %w", err)
	}
	result, err := fn(NewStore(tx))
	if err != nil {
		return Detail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit idempotent parcel request: %w", err)
	}
	return result, nil
}
func (s *PostgresStore) ReplayCustomer(ctx context.Context, user, key string) (repository.Parcel, error) {
	if s.db == nil {
		return repository.Parcel{}, ErrUnavailable
	}
	r, err := repository.New(s.db)
	if err != nil {
		return repository.Parcel{}, err
	}
	return r.ParcelByCustomerIdempotencyKey(ctx, user, key)
}
func (s *PostgresStore) ReplayMerchant(ctx context.Context, merchant, order string) (repository.Parcel, error) {
	if s.db == nil {
		return repository.Parcel{}, ErrUnavailable
	}
	r, err := repository.New(s.db)
	if err != nil {
		return repository.Parcel{}, err
	}
	return r.ParcelByMerchantExternalOrderID(ctx, merchant, order)
}
func (s *PostgresStore) Create(ctx context.Context, p repository.CreateParcelParams, role string) (repository.Parcel, error) {
	if s.db == nil {
		return repository.Parcel{}, ErrUnavailable
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repository.Parcel{}, fmt.Errorf("begin parcel creation: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	r, err := repository.New(tx)
	if err != nil {
		return repository.Parcel{}, err
	}
	parcel, err := r.CreateParcel(ctx, p)
	if err != nil {
		return repository.Parcel{}, err
	}
	historyID, err := uuid()
	if err != nil {
		return repository.Parcel{}, err
	}
	if _, err = r.AppendStatusHistory(ctx, repository.AppendStatusHistoryParams{ID: historyID, ParcelID: parcel.ID, NewStatus: repository.ParcelStatusCreated, ActorUserID: &p.CreatedByUserID, ActorRole: &role}); err != nil {
		return repository.Parcel{}, err
	}
	eventID, err := uuid()
	if err != nil {
		return repository.Parcel{}, err
	}
	data := map[string]any{"parcel_id": parcel.ID, "tracking_number": parcel.TrackingNumber, "status": "CREATED"}
	if parcel.CustomerUserID != nil {
		data["customer_user_id"] = *parcel.CustomerUserID
	} else if parcel.MerchantID != nil {
		data["merchant_id"] = *parcel.MerchantID
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return repository.Parcel{}, err
	}
	// The relay maps these agreed storage labels to parcel.created.v1 and
	// wraps this data object in the canonical envelope after commit.
	_, err = tx.Exec(ctx, `INSERT INTO parcel.outbox_events (id,event_type,routing_key,payload,aggregate_type,aggregate_id) VALUES ($1,'parcel.created','parcel.created',$2,'parcel',$3)`, eventID, payload, parcel.ID)
	if err != nil {
		return repository.Parcel{}, fmt.Errorf("insert parcel outbox: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return repository.Parcel{}, fmt.Errorf("commit parcel creation: %w", err)
	}
	return parcel, nil
}
func (s *PostgresStore) Owned(ctx context.Context, id, user string, staff bool) (repository.Parcel, error) {
	if s.db == nil {
		return repository.Parcel{}, ErrUnavailable
	}
	r, err := repository.New(s.db)
	if err != nil {
		return repository.Parcel{}, err
	}
	return r.ParcelByViewer(ctx, id, user, staff)
}
func (s *PostgresStore) History(ctx context.Context, id string) ([]repository.StatusHistory, error) {
	if s.db == nil {
		return nil, ErrUnavailable
	}
	r, err := repository.New(s.db)
	if err != nil {
		return nil, err
	}
	return r.StatusHistoryByParcel(ctx, id)
}
func (s *PostgresStore) List(ctx context.Context, user string, f Filter) ([]repository.Parcel, int, error) {
	if s.db == nil {
		return nil, 0, ErrUnavailable
	}
	r, err := repository.New(s.db)
	if err != nil {
		return nil, 0, err
	}
	var to *time.Time
	if f.To != nil {
		end := f.To.AddDate(0, 0, 1)
		to = &end
	}
	return r.CustomerParcels(ctx, user, f.Status, f.From, to, f.Page, f.PageSize, f.Sort == "createdAt:asc")
}
func numeric(d Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(d)), Exp: -2, Valid: true}
}
func numericDecimal(n pgtype.Numeric) (Decimal, error) {
	if !n.Valid || n.NaN || n.InfinityModifier != 0 || n.Int == nil {
		return 0, errors.New("invalid stored decimal")
	}
	r := new(big.Rat).SetInt(n.Int)
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs32(n.Exp))), nil)
	if n.Exp < 0 {
		r.Quo(r, new(big.Rat).SetInt(factor))
	} else {
		r.Mul(r, new(big.Rat).SetInt(factor))
	}
	r.Mul(r, big.NewRat(100, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, errors.New("invalid stored precision")
	}
	return Decimal(r.Num().Int64()), nil
}
func abs32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}
func createParams(customer, merchant, external *string, key, actor string, in CreateInput, fee Fee) (repository.CreateParcelParams, error) {
	id, err := uuid()
	if err != nil {
		return repository.CreateParcelParams{}, err
	}
	raw, err := json.Marshal(breakdown{fee.BaseFee, fee.WeightCharge, fee.SizeSurcharge})
	if err != nil {
		return repository.CreateParcelParams{}, err
	}
	var k *string
	if key != "" {
		k = &key
	}
	return repository.CreateParcelParams{ID: id, CustomerUserID: customer, MerchantID: merchant, ExternalOrderID: external, IdempotencyKey: k, SenderName: in.Sender.Name, SenderPhone: in.Sender.Phone, PickupAddress: in.Sender.Address, PickupCity: in.Sender.City, PickupZoneID: in.Sender.ZoneID, RecipientName: in.Recipient.Name, RecipientPhone: in.Recipient.Phone, DropoffAddress: in.Recipient.Address, DropoffCity: in.Recipient.City, DropoffZoneID: in.Recipient.ZoneID, WeightKG: numeric(in.WeightKG), SizeCategory: repository.SizeCategory(in.SizeCategory), Description: in.Description, PriceAmount: numeric(fee.Total), Currency: fee.Currency, PriceBreakdown: raw, CreatedByUserID: actor}, nil
}
func detail(p repository.Parcel) (Detail, error) {
	weight, err := numericDecimal(p.WeightKG)
	if err != nil {
		return Detail{}, err
	}
	total, err := numericDecimal(p.PriceAmount)
	if err != nil {
		return Detail{}, err
	}
	fee, err := storedFee(p.PriceBreakdown, p.Currency, total)
	if err != nil {
		return Detail{}, err
	}
	return Detail{ID: p.ID, TrackingNumber: p.TrackingNumber, Status: string(p.Status), Sender: Contact{p.SenderName, p.SenderPhone, p.PickupAddress, p.PickupCity, p.PickupZoneID}, Recipient: Contact{p.RecipientName, p.RecipientPhone, p.DropoffAddress, p.DropoffCity, p.DropoffZoneID}, WeightKG: weight, SizeCategory: string(p.SizeCategory), Description: p.Description, Fee: fee, FailureReason: p.FailureReason, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, DeliveredAt: p.DeliveredAt}, nil
}
