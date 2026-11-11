package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/businessreg"
	"github.com/jackc/pgx/v5"
)

type MerchantStatus string

const (
	MerchantStatusPending   MerchantStatus = "PENDING"
	MerchantStatusApproved  MerchantStatus = "APPROVED"
	MerchantStatusRejected  MerchantStatus = "REJECTED"
	MerchantStatusSuspended MerchantStatus = "SUSPENDED"
)

type Merchant struct {
	ID                string
	OwnerUserID       string
	BusinessName      string
	BusinessRegNo     *string
	BusinessAddress   *string
	ContactEmail      string
	ContactPhone      string
	PickupAddress     string
	PickupZoneID      string
	Status            MerchantStatus
	ApprovedBy        *string
	ApprovedAt        *time.Time
	LifecycleSequence int64
	RejectionReason   *string
	SuspensionReason  *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateMerchantParams struct {
	ID              string
	OwnerUserID     string
	BusinessName    string
	BusinessRegNo   string
	BusinessAddress string
	ContactEmail    string
	ContactPhone    string
	PickupAddress   string
	PickupZoneID    string
}

type ProfileUpdate struct {
	ContactEmail  string
	ContactPhone  string
	PickupAddress string
	PickupZoneID  string
}

func (r *Repository) CreateMerchant(ctx context.Context, params CreateMerchantParams) error {
	businessRegNo, err := businessreg.NormalizeValid(params.BusinessRegNo)
	if err != nil {
		return ErrInvalidRegistrationNumber
	}
	_, err = r.db.Exec(
		ctx,
		`INSERT INTO merchant.merchants (
			id, owner_user_id, business_name, business_reg_no, business_address, contact_email,
			contact_phone, pickup_address, pickup_zone_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		params.ID,
		params.OwnerUserID,
		params.BusinessName,
		businessRegNo,
		nullable(params.BusinessAddress),
		params.ContactEmail,
		params.ContactPhone,
		params.PickupAddress,
		params.PickupZoneID,
	)
	if err != nil {
		return fmt.Errorf("creating merchant: %w", err)
	}

	return nil
}

const merchantColumns = `id, owner_user_id, business_name, business_reg_no, business_address, contact_email,
	contact_phone, pickup_address, pickup_zone_id, status, approved_by,
	approved_at, lifecycle_sequence, rejection_reason, suspension_reason, created_at, updated_at`

func scanMerchant(row pgx.Row) (Merchant, error) {
	merchant := Merchant{}
	err := row.Scan(
		&merchant.ID, &merchant.OwnerUserID, &merchant.BusinessName, &merchant.BusinessRegNo,
		&merchant.BusinessAddress, &merchant.ContactEmail, &merchant.ContactPhone,
		&merchant.PickupAddress, &merchant.PickupZoneID, &merchant.Status,
		&merchant.ApprovedBy, &merchant.ApprovedAt, &merchant.LifecycleSequence,
		&merchant.RejectionReason, &merchant.SuspensionReason, &merchant.CreatedAt, &merchant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Merchant{}, ErrNotFound
	}
	if err != nil {
		return Merchant{}, fmt.Errorf("querying merchant: %w", err)
	}
	return merchant, nil
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func (r *Repository) MerchantByID(ctx context.Context, merchantID string) (Merchant, error) {
	return scanMerchant(r.db.QueryRow(ctx, `SELECT `+merchantColumns+` FROM merchant.merchants WHERE id = $1`, merchantID))
}

// MerchantByOwnerUserID resolves the merchant owned by an authenticated user.
func (r *Repository) MerchantByOwnerUserID(ctx context.Context, ownerUserID string) (Merchant, error) {
	return scanMerchant(r.db.QueryRow(ctx, `SELECT `+merchantColumns+` FROM merchant.merchants WHERE owner_user_id = $1`, ownerUserID))
}

// MerchantByContactEmail resolves a merchant by its business contact email.
// The baseline schema does not make contact_email unique, so the oldest match
// wins and callers that require a single deterministic owner must treat a
// different business registration number as a conflict.
func (r *Repository) MerchantByContactEmail(ctx context.Context, contactEmail string) (Merchant, error) {
	return scanMerchant(r.db.QueryRow(ctx, `SELECT `+merchantColumns+` FROM merchant.merchants WHERE lower(contact_email) = lower($1) ORDER BY created_at, id LIMIT 1`, strings.TrimSpace(contactEmail)))
}

// UpdateProfile changes only merchant-editable contact and pickup fields.
func (r *Repository) UpdateProfile(ctx context.Context, merchantID string, update ProfileUpdate) (Merchant, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE merchant.merchants
		SET contact_email = $2, contact_phone = $3, pickup_address = $4,
		    pickup_zone_id = $5, updated_at = now()
		WHERE id = $1`, merchantID, update.ContactEmail, update.ContactPhone, update.PickupAddress, update.PickupZoneID)
	if err != nil {
		return Merchant{}, fmt.Errorf("updating merchant profile: %w", err)
	}
	return r.MerchantByID(ctx, merchantID)
}

func (r *Repository) ApproveMerchant(ctx context.Context, merchantID, actorUserID string) (Merchant, error) {
	return r.transition(ctx, merchantID, actorUserID, MerchantStatusApproved, "APPROVED", "")
}

func (r *Repository) RejectMerchant(ctx context.Context, merchantID, actorUserID, reason string) (Merchant, error) {
	return r.transition(ctx, merchantID, actorUserID, MerchantStatusRejected, "REJECTED", reason)
}

func (r *Repository) SuspendMerchant(ctx context.Context, merchantID, actorUserID, reason string) (Merchant, error) {
	return r.transition(ctx, merchantID, actorUserID, MerchantStatusSuspended, "SUSPENDED", reason)
}

func (r *Repository) transition(ctx context.Context, merchantID, actorUserID string, target MerchantStatus, action, reason string) (Merchant, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Merchant{}, fmt.Errorf("begin merchant transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current MerchantStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM merchant.merchants WHERE id = $1 FOR UPDATE`, merchantID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, ErrNotFound
		}
		return Merchant{}, fmt.Errorf("lock merchant: %w", err)
	}
	if !validTransition(current, target) {
		return Merchant{}, fmt.Errorf("%w from %s to %s", ErrInvalidState, current, target)
	}

	var update string
	args := []any{target, merchantID}
	switch target {
	case MerchantStatusApproved:
		update = `UPDATE merchant.merchants SET status = $1, approved_by = $3, approved_at = now(), rejection_reason = NULL, suspension_reason = NULL, updated_at = now() WHERE id = $2`
		args = append(args, actorUserID)
	case MerchantStatusRejected:
		update = `UPDATE merchant.merchants SET status = $1, rejection_reason = $3, updated_at = now() WHERE id = $2`
		args = append(args, reason)
	case MerchantStatusSuspended:
		update = `UPDATE merchant.merchants SET status = $1, suspension_reason = $3, updated_at = now() WHERE id = $2`
		args = append(args, reason)
	}
	if _, err := tx.Exec(ctx, update, args...); err != nil {
		return Merchant{}, fmt.Errorf("update merchant state: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant.audit_logs (id, actor_user_id, action, entity_type, entity_id, details) VALUES (gen_random_uuid(), $1, $2, 'merchant', $3, jsonb_build_object('reason', NULLIF($4, '')))`, actorUserID, action, merchantID, reason); err != nil {
		return Merchant{}, fmt.Errorf("record merchant audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, fmt.Errorf("commit merchant transition: %w", err)
	}
	return r.MerchantByID(ctx, merchantID)
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func validTransition(from, to MerchantStatus) bool {
	switch to {
	case MerchantStatusApproved:
		return from == MerchantStatusPending || from == MerchantStatusSuspended
	case MerchantStatusRejected:
		return from == MerchantStatusPending
	case MerchantStatusSuspended:
		return from == MerchantStatusApproved
	default:
		return false
	}
}
