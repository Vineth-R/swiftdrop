package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
)

type connectionPool interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// WithIdempotencyLock serialises attempts that share a merchant and
// Idempotency-Key. The lock is session-scoped on a dedicated pooled connection.
// If the process dies the connection closes and PostgreSQL releases the lock.
//
// The lock is held for the whole attempt, including the external Parcel call.
// That is intentional: it lets a same-payload waiter replay the committed
// order instead of reaching Parcel, and it lets a different-payload waiter be
// rejected before Parcel. Parcel has its own short timeout, so the connection is
// not held indefinitely.
//
// Waiting uses pg_try_advisory_lock with a short backoff. A blocking
// pg_advisory_lock would hold a pooled connection while waiting, and a waiter
// holding a connection could starve the lock holder's own reservation and
// order queries when the pool is small. The backoff leaves the pool available
// to the holder.
func (r *Repository) WithIdempotencyLock(ctx context.Context, merchantID, idempotencyKey string, fn func(context.Context) error) error {
	if idempotencyKey == "" {
		return fn(ctx)
	}
	pool, ok := r.db.(connectionPool)
	if !ok {
		return errors.New("merchant order idempotency lock requires a connection pool")
	}
	lockKey := advisoryLockKey(merchantID, idempotencyKey)
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var conn *pgxpool.Conn
	for {
		candidate, err := pool.Acquire(waitCtx)
		if err != nil {
			return fmt.Errorf("acquire idempotency lock connection: %w", err)
		}
		var acquired bool
		if err := candidate.QueryRow(waitCtx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired); err != nil {
			candidate.Release()
			return fmt.Errorf("acquire merchant order idempotency lock: %w", err)
		}
		if acquired {
			conn = candidate
			break
		}
		candidate.Release()
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return fmt.Errorf("wait for merchant order idempotency lock: %w", waitCtx.Err())
		case <-timer.C:
		}
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, lockKey); err != nil {
			// Never return a connection that may still hold the lock to the pool.
			if raw := conn.Hijack(); raw != nil {
				_ = raw.Close(context.Background())
			}
			return
		}
		conn.Release()
	}()
	return fn(ctx)
}

func advisoryLockKey(merchantID, idempotencyKey string) int64 {
	digest := sha256.Sum256([]byte(merchantID + "\x00" + idempotencyKey))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

// ReserveOrderAttempt creates or resumes the reservation for one attempt. A
// different request hash for the same key is rejected before any external call.
func (r *Repository) ReserveOrderAttempt(ctx context.Context, merchantID, idempotencyKey, requestHash string) (orders.OrderReservation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return orders.OrderReservation{}, fmt.Errorf("begin order reservation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := scanOrderReservation(tx.QueryRow(ctx, `
		INSERT INTO merchant.merchant_order_reservations
			(merchant_id, idempotency_key, request_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (merchant_id, idempotency_key) DO NOTHING
		RETURNING id, merchant_id, idempotency_key, request_hash, status, order_id::text`,
		merchantID, idempotencyKey, requestHash))
	if errors.Is(err, ErrNotFound) {
		reservation, err = scanOrderReservation(tx.QueryRow(ctx, `
			SELECT id, merchant_id, idempotency_key, request_hash, status, order_id::text
			FROM merchant.merchant_order_reservations
			WHERE merchant_id = $1 AND idempotency_key = $2
			FOR UPDATE`, merchantID, idempotencyKey))
		if err != nil {
			return orders.OrderReservation{}, fmt.Errorf("load order reservation: %w", err)
		}
		if reservation.RequestHash != requestHash {
			return orders.OrderReservation{}, orders.ErrDuplicateOrder
		}
		if reservation.Status != orders.OrderReservationCompleted {
			if _, err := tx.Exec(ctx, `
				UPDATE merchant.merchant_order_reservations
				SET status = 'PENDING', last_error = NULL
				WHERE id = $1`, reservation.ID); err != nil {
				return orders.OrderReservation{}, fmt.Errorf("resume order reservation: %w", err)
			}
			reservation.Status = orders.OrderReservationPending
		}
	} else if err != nil {
		return orders.OrderReservation{}, fmt.Errorf("create order reservation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return orders.OrderReservation{}, fmt.Errorf("commit order reservation: %w", err)
	}
	return reservation, nil
}

// CompleteOrderAttempt records the order that satisfied the reservation.
func (r *Repository) CompleteOrderAttempt(ctx context.Context, reservationID, orderID string) error {
	if strings.TrimSpace(reservationID) == "" {
		return nil
	}
	result, err := r.db.Exec(ctx, `
		UPDATE merchant.merchant_order_reservations
		SET status = 'COMPLETED', order_id = $2, last_error = NULL
		WHERE id = $1`, reservationID, orderID)
	if err != nil {
		return fmt.Errorf("complete order reservation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FailOrderAttempt keeps the request hash for future retries while recording
// that the attempt did not commit an order.
func (r *Repository) FailOrderAttempt(ctx context.Context, reservationID, reason string) error {
	if strings.TrimSpace(reservationID) == "" {
		return nil
	}
	// A cancelled request still needs to leave a retryable FAILED record. The
	// update is short and does not depend on the caller's cancellation.
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := r.db.Exec(failCtx, `
		UPDATE merchant.merchant_order_reservations
		SET status = 'FAILED', last_error = NULLIF($2, '')
		WHERE id = $1 AND status <> 'COMPLETED'`, reservationID, truncateReason(reason))
	if err != nil {
		return fmt.Errorf("fail order reservation: %w", err)
	}
	return nil
}

func scanOrderReservation(row pgx.Row) (orders.OrderReservation, error) {
	var reservation orders.OrderReservation
	err := row.Scan(&reservation.ID, &reservation.MerchantID, &reservation.IdempotencyKey,
		&reservation.RequestHash, &reservation.Status, &reservation.OrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return orders.OrderReservation{}, ErrNotFound
	}
	if err != nil {
		return orders.OrderReservation{}, err
	}
	return reservation, nil
}

func truncateReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 255 {
		return reason[:255]
	}
	return reason
}
