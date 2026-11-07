package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ParcelByViewer authorizes ownership in the same query that loads the record.
func (r *Repository) ParcelByViewer(ctx context.Context, id, user string, staff bool) (Parcel, error) {
	p, err := scanParcel(r.db.QueryRow(ctx, `SELECT `+parcelColumns+` FROM parcel.parcels WHERE id=$1 AND ($3 OR customer_user_id::text=$2)`, id, user, staff))
	if err != nil {
		return Parcel{}, fmt.Errorf("querying authorized parcel: %w", databaseError(err))
	}
	return p, nil
}

type countedRow struct {
	row   pgx.Row
	total *int
}

func (r countedRow) Scan(dest ...any) error { return r.row.Scan(append(dest, r.total)...) }

func (r *Repository) CustomerParcels(ctx context.Context, user, status string, from, to *time.Time, page, pageSize int, ascending bool) ([]Parcel, int, error) {
	order := "DESC"
	if ascending {
		order = "ASC"
	}
	// order is a fixed allowlisted SQL fragment; all user data is parameterized.
	where := ` FROM parcel.parcels WHERE customer_user_id=$1 AND ($2='' OR status=$2) AND ($3::timestamptz IS NULL OR created_at >= $3) AND ($4::timestamptz IS NULL OR created_at < $4)`
	rows, err := r.db.Query(ctx, `SELECT `+parcelColumns+`,count(*) OVER()`+where+` ORDER BY created_at `+order+`,id `+order+` LIMIT $5 OFFSET $6`, user, status, from, to, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("querying customer parcels: %w", databaseError(err))
	}
	out := make([]Parcel, 0)
	total := 0
	for rows.Next() {
		p, err := scanParcel(countedRow{rows, &total})
		if err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("scanning customer parcels: %w", databaseError(err))
		}
		out = append(out, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("iterating customer parcels: %w", databaseError(err))
	}
	if len(out) == 0 {
		if err := r.db.QueryRow(ctx, `SELECT count(*)`+where, user, status, from, to).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting customer parcels: %w", databaseError(err))
		}
	}
	return out, total, nil
}
