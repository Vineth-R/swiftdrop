package repository

import (
	"context"
	"fmt"
	"time"
)

// ParcelAssignment is one row of parcel.parcel_assignments, oldest first when
// read through ParcelAssignments. Courier and actor IDs are Identity user IDs.
type ParcelAssignment struct {
	ID                string
	ParcelID          string
	CourierID         string
	PreviousCourierID *string
	AssignedBy        string
	AssignedAt        time.Time
	UnassignedAt      *time.Time
	IsCurrent         bool
	Reason            *string
}

// ParcelAssignments returns the assignment history of one parcel, oldest
// first. It is append-only by construction: a reassignment closes the previous
// row and inserts a new current row.
func (r *Repository) ParcelAssignments(ctx context.Context, parcelID string) ([]ParcelAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, parcel_id, courier_id, previous_courier_id, assigned_by,
		       assigned_at, unassigned_at, is_current, reason
		FROM parcel.parcel_assignments
		WHERE parcel_id = $1
		ORDER BY assigned_at, id`, parcelID)
	if err != nil {
		return nil, fmt.Errorf("querying parcel assignments: %w", databaseError(err))
	}
	defer rows.Close()

	assignments := make([]ParcelAssignment, 0)
	for rows.Next() {
		var assignment ParcelAssignment
		if err := rows.Scan(
			&assignment.ID,
			&assignment.ParcelID,
			&assignment.CourierID,
			&assignment.PreviousCourierID,
			&assignment.AssignedBy,
			&assignment.AssignedAt,
			&assignment.UnassignedAt,
			&assignment.IsCurrent,
			&assignment.Reason,
		); err != nil {
			return nil, fmt.Errorf("scanning parcel assignment: %w", databaseError(err))
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating parcel assignments: %w", databaseError(err))
	}
	return assignments, nil
}
