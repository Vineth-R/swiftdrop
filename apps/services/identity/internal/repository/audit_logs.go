package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AuditLogFilter is the validated filter for the ADMIN audit-log read API.
// All values travel to PostgreSQL as parameters.
type AuditLogFilter struct {
	ActorUserID string
	Action      string
	EntityType  string
	EntityID    string
	From        *time.Time
	To          *time.Time
	Limit       int
	Offset      int
	Ascending   bool
}

// AuditLog is one identity.audit_logs row. Details is already a bounded JSON
// object written by this service; the HTTP layer applies a final redaction pass.
type AuditLog struct {
	ID          string
	ActorUserID string
	Action      string
	EntityType  string
	EntityID    *string
	Details     json.RawMessage
	CreatedAt   time.Time
}

// ListAuditLogs returns one page of audit rows and the total number of matches.
func (r *Repository) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AuditLog, int, error) {
	where, args := auditWhere(filter)

	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM identity.audit_logs`+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	direction := "DESC"
	if filter.Ascending {
		direction = "ASC"
	}
	args = append(args, filter.Limit, filter.Offset)
	query := `
		SELECT id, actor_user_id, action, entity_type, entity_id::text, details, created_at
		FROM identity.audit_logs` + where +
		" ORDER BY created_at " + direction + ", id " + direction +
		" LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audit logs: %w", err)
	}
	defer rows.Close()

	logs := []AuditLog{}
	for rows.Next() {
		var entry AuditLog
		if err := rows.Scan(
			&entry.ID, &entry.ActorUserID, &entry.Action, &entry.EntityType,
			&entry.EntityID, &entry.Details, &entry.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit log: %w", err)
		}
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading audit logs: %w", err)
	}
	return logs, total, nil
}

func auditWhere(filter AuditLogFilter) (string, []any) {
	var (
		conditions []string
		args       []any
	)
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, strings.ReplaceAll(condition, "?", "$"+strconv.Itoa(len(args))))
	}
	if filter.ActorUserID != "" {
		add("actor_user_id = ?::uuid", filter.ActorUserID)
	}
	if filter.Action != "" {
		add("action = ?", filter.Action)
	}
	if filter.EntityType != "" {
		add("entity_type = ?", filter.EntityType)
	}
	if filter.EntityID != "" {
		add("entity_id = ?::uuid", filter.EntityID)
	}
	if filter.From != nil {
		add("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		add("created_at < ?", *filter.To)
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
