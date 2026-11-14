//go:build integration

package database_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// apiTransitions mirrors API v1.1 section 6.3. It is intentionally duplicated
// here instead of importing the parcel service's internal state machine so the
// integration package stays decoupled from service internals. The unit tests
// assert the Go state machine against the same matrix.
var apiTransitions = map[string][]string{
	"CREATED":          {"PICKED_UP", "FAILED"},
	"PICKED_UP":        {"IN_TRANSIT", "FAILED"},
	"IN_TRANSIT":       {"OUT_FOR_DELIVERY", "FAILED"},
	"OUT_FOR_DELIVERY": {"DELIVERED", "FAILED"},
	"DELIVERED":        {},
	"FAILED":           {},
}

func apiParcelStatuses() []string {
	return []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"}
}

func apiCanTransition(from, to string) bool {
	for _, allowed := range apiTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

func TestParcelDispatchSchema(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 3)

	assignmentColumns := map[string]struct {
		dataType string
		nullable string
	}{
		"id":                  {"uuid", "NO"},
		"parcel_id":           {"uuid", "NO"},
		"courier_id":          {"uuid", "NO"},
		"assigned_by":         {"uuid", "NO"},
		"assigned_at":         {"timestamp with time zone", "NO"},
		"unassigned_at":       {"timestamp with time zone", "YES"},
		"is_current":          {"boolean", "NO"},
		"reason":              {"character varying", "YES"},
		"previous_courier_id": {"uuid", "YES"},
		"updated_at":          {"timestamp with time zone", "NO"},
	}
	for column, want := range assignmentColumns {
		dataType, nullable := columnType(t, ctx, conn, "parcel_assignments", column)
		if dataType != want.dataType || nullable != want.nullable {
			t.Errorf("parcel_assignments.%s = (%s, nullable=%s); want (%s, nullable=%s)",
				column, dataType, nullable, want.dataType, want.nullable)
		}
	}

	outboxColumns := map[string]struct {
		dataType string
		nullable string
	}{
		"id":             {"uuid", "NO"},
		"event_type":     {"character varying", "NO"},
		"routing_key":    {"character varying", "NO"},
		"payload":        {"jsonb", "NO"},
		"status":         {"character varying", "NO"},
		"attempts":       {"integer", "NO"},
		"last_error":     {"character varying", "YES"},
		"created_at":     {"timestamp with time zone", "NO"},
		"published_at":   {"timestamp with time zone", "YES"},
		"aggregate_type": {"character varying", "YES"},
		"aggregate_id":   {"uuid", "YES"},
	}
	for column, want := range outboxColumns {
		dataType, nullable := columnType(t, ctx, conn, "outbox_events", column)
		if dataType != want.dataType || nullable != want.nullable {
			t.Errorf("outbox_events.%s = (%s, nullable=%s); want (%s, nullable=%s)",
				column, dataType, nullable, want.dataType, want.nullable)
		}
	}

	for _, index := range []struct {
		table string
		name  string
	}{
		{"parcel_assignments", "assignments_one_current_uq"},
		{"parcel_assignments", "assignments_courier_idx"},
		{"outbox_events", "outbox_status_created"},
		{"outbox_events", "outbox_aggregate_idx"},
	} {
		if !indexExists(t, ctx, conn, index.table, index.name) {
			t.Errorf("missing index %s on parcel.%s", index.name, index.table)
		}
	}

	for _, constraint := range []struct {
		table string
		name  string
	}{
		{"parcel_assignments", "assignments_closed_has_time"},
		{"outbox_events", "outbox_type_key_pair"},
		{"outbox_events", "outbox_aggregate_pair"},
	} {
		if !constraintExists(t, ctx, conn, constraint.table, constraint.name) {
			t.Errorf("missing constraint %s on parcel.%s", constraint.name, constraint.table)
		}
	}

	for _, trigger := range []struct {
		table string
		name  string
	}{
		{"parcel_assignments", "set_updated_at"},
		{"parcels", "parcels_status_transition"},
		{"parcels", "set_updated_at"},
		{"parcel_status_history", "history_append_only"},
	} {
		if !triggerExists(t, ctx, conn, trigger.table, trigger.name) {
			t.Errorf("missing trigger %s on parcel.%s", trigger.name, trigger.table)
		}
	}
}

func TestParcelAssignmentLifecycle(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 3)

	parcelID := insertParcel(t, ctx, conn, "CREATED")
	firstCourier := newUUID(t, ctx, conn)
	secondCourier := newUUID(t, ctx, conn)

	firstAssignment := insertAssignment(t, ctx, conn, parcelID, firstCourier, nil, true)

	_, duplicateErr := conn.Exec(ctx, `
		INSERT INTO parcel.parcel_assignments (parcel_id, courier_id, assigned_by)
		VALUES ($1, $2, gen_random_uuid())`, parcelID, secondCourier)
	assertSQLState(t, duplicateErr, "23505")

	if _, err := conn.Exec(ctx, `
		UPDATE parcel.parcel_assignments SET updated_at = now() - interval '1 hour' WHERE id = $1`,
		firstAssignment); err != nil {
		t.Fatalf("age first assignment: %v", err)
	}
	var beforeClose time.Time
	if err := conn.QueryRow(ctx, `SELECT updated_at FROM parcel.parcel_assignments WHERE id = $1`, firstAssignment).
		Scan(&beforeClose); err != nil {
		t.Fatalf("read first assignment timestamp: %v", err)
	}

	if _, err := conn.Exec(ctx, `
		UPDATE parcel.parcel_assignments
		SET is_current = false, unassigned_at = now()
		WHERE id = $1`, firstAssignment); err != nil {
		t.Fatalf("close first assignment: %v", err)
	}

	var afterClose time.Time
	if err := conn.QueryRow(ctx, `SELECT updated_at FROM parcel.parcel_assignments WHERE id = $1`, firstAssignment).
		Scan(&afterClose); err != nil {
		t.Fatalf("read closed assignment timestamp: %v", err)
	}
	if !afterClose.After(beforeClose) {
		t.Errorf("updated_at did not advance on close: before=%s after=%s", beforeClose, afterClose)
	}

	secondAssignment := insertAssignment(t, ctx, conn, parcelID, secondCourier, &firstCourier, true)

	var totalAssignments, currentAssignments int
	if err := conn.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE is_current)
		FROM parcel.parcel_assignments WHERE parcel_id = $1`, parcelID).
		Scan(&totalAssignments, &currentAssignments); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if totalAssignments != 2 || currentAssignments != 1 {
		t.Errorf("assignments = %d total / %d current; want 2 total / 1 current", totalAssignments, currentAssignments)
	}

	var previousCourier, reason string
	if err := conn.QueryRow(ctx, `
		SELECT previous_courier_id::text, reason
		FROM parcel.parcel_assignments WHERE id = $1`, secondAssignment).
		Scan(&previousCourier, &reason); err != nil {
		t.Fatalf("read reassignment metadata: %v", err)
	}
	if previousCourier != firstCourier {
		t.Errorf("previous_courier_id = %s; want %s", previousCourier, firstCourier)
	}
	if reason != "test assignment" {
		t.Errorf("reason = %q; want %q", reason, "test assignment")
	}

	var activeForSecond, activeForFirst int
	if err := conn.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE courier_id = $1 AND is_current),
			count(*) FILTER (WHERE courier_id = $2 AND is_current)
		FROM parcel.parcel_assignments WHERE parcel_id = $3`,
		secondCourier, firstCourier, parcelID).Scan(&activeForSecond, &activeForFirst); err != nil {
		t.Fatalf("query courier workload: %v", err)
	}
	if activeForSecond != 1 || activeForFirst != 0 {
		t.Errorf("active assignments second/first = %d/%d; want 1/0", activeForSecond, activeForFirst)
	}

	_, err := conn.Exec(ctx, `
		INSERT INTO parcel.parcel_assignments (parcel_id, courier_id, assigned_by)
		VALUES (gen_random_uuid(), $1, gen_random_uuid())`, firstCourier)
	assertSQLState(t, err, "23503")
}

func TestParcelStatusTriggerMatchesAPIMatrix(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 3)

	for _, from := range apiParcelStatuses() {
		for _, to := range apiParcelStatuses() {
			name := fmt.Sprintf("%s_to_%s", from, to)
			t.Run(name, func(t *testing.T) {
				parcelID := insertParcel(t, ctx, conn, from)
				_, err := conn.Exec(ctx, `
					UPDATE parcel.parcels SET
						status = $2::text,
						failure_reason = CASE WHEN $2::text = 'FAILED' THEN 'Recipient unavailable' ELSE failure_reason END,
						failed_at = CASE WHEN $2::text = 'FAILED' THEN now() ELSE failed_at END,
						delivered_at = CASE WHEN $2::text = 'DELIVERED' THEN now() ELSE delivered_at END,
						delivered_to_name = CASE WHEN $2::text = 'DELIVERED' THEN 'Amali Fernando' ELSE delivered_to_name END
					WHERE id = $1`, parcelID, to)

				allowedByDomain := apiCanTransition(from, to)
				sameState := from == to
				if sameState {
					// The database trigger only fires when the status changes, so a
					// same-status update is a permitted no-op even though the domain
					// state machine rejects it as a transition.
					if err != nil {
						t.Fatalf("same-status update %s -> %s failed: %v", from, to, err)
					}
				} else if allowedByDomain {
					if err != nil {
						t.Fatalf("allowed transition %s -> %s failed: %v", from, to, err)
					}
				} else {
					assertSQLState(t, err, "23514")
				}

				var storedStatus string
				if err := conn.QueryRow(ctx, `SELECT status FROM parcel.parcels WHERE id = $1`, parcelID).
					Scan(&storedStatus); err != nil {
					t.Fatalf("read stored status: %v", err)
				}
				wantStatus := string(from)
				if allowedByDomain && !sameState {
					wantStatus = string(to)
				}
				if storedStatus != wantStatus {
					t.Errorf("stored status = %s; want %s", storedStatus, wantStatus)
				}
			})
		}
	}
}

func TestParcelHistoryIsAppendOnly(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 3)

	parcelID := insertParcel(t, ctx, conn, "CREATED")
	var historyID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO parcel.parcel_status_history (parcel_id, previous_status, new_status, actor_role)
		VALUES ($1, NULL, 'CREATED', 'COURIER') RETURNING id`, parcelID).Scan(&historyID); err != nil {
		t.Fatalf("insert history: %v", err)
	}

	_, updateErr := conn.Exec(ctx, `UPDATE parcel.parcel_status_history SET note = 'changed' WHERE id = $1`, historyID)
	if updateErr == nil {
		t.Fatal("updating a history row succeeded; want append-only rejection")
	}

	_, deleteErr := conn.Exec(ctx, `DELETE FROM parcel.parcel_status_history WHERE id = $1`, historyID)
	if deleteErr == nil {
		t.Fatal("deleting a history row succeeded; want append-only rejection")
	}
}

func TestOutboxFoundation(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 3)

	parcelID := insertParcel(t, ctx, conn, "PICKED_UP")

	var eventID string
	if err := conn.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&eventID); err != nil {
		t.Fatalf("generate event id: %v", err)
	}
	payload := fmt.Sprintf(`{
		"eventId": %q,
		"eventType": "parcel.status_changed",
		"occurredAt": "2026-11-04T13:00:00Z",
		"schemaVersion": 1,
		"parcelId": %q,
		"previousStatus": "PICKED_UP",
		"status": "IN_TRANSIT"
	}`, eventID, parcelID)

	var storedID, storedStatus, payloadEventType string
	var attempts int
	var publishedAt *time.Time
	if err := conn.QueryRow(ctx, `
		INSERT INTO parcel.outbox_events (
			id, event_type, routing_key, payload, aggregate_type, aggregate_id
		) VALUES ($1, 'parcel.status_changed', 'parcel.status.changed', $2::jsonb, 'parcel', $3)
		RETURNING id::text, status, attempts, published_at, payload->>'eventType'`,
		eventID, payload, parcelID,
	).Scan(&storedID, &storedStatus, &attempts, &publishedAt, &payloadEventType); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	if storedID != eventID || storedStatus != "PENDING" || attempts != 0 || publishedAt != nil {
		t.Errorf("new event = id %s status %s attempts %d published %v; want id %s PENDING 0 nil",
			storedID, storedStatus, attempts, publishedAt, eventID)
	}
	if payloadEventType != "parcel.status_changed" {
		t.Errorf("payload eventType = %q; want parcel.status_changed", payloadEventType)
	}

	var pending int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM parcel.outbox_events
		WHERE status = 'PENDING' AND aggregate_type = 'parcel' AND aggregate_id = $1`, parcelID).
		Scan(&pending); err != nil {
		t.Fatalf("query pending events: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending events = %d; want 1", pending)
	}

	if _, err := conn.Exec(ctx, `
		UPDATE parcel.outbox_events
		SET status = 'PUBLISHED', published_at = now(), attempts = attempts + 1
		WHERE id = $1`, eventID); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	var publishedStatus string
	var published time.Time
	if err := conn.QueryRow(ctx, `SELECT status, published_at FROM parcel.outbox_events WHERE id = $1`, eventID).
		Scan(&publishedStatus, &published); err != nil {
		t.Fatalf("read published event: %v", err)
	}
	if publishedStatus != "PUBLISHED" || published.IsZero() {
		t.Errorf("published event = %s at %v; want PUBLISHED with timestamp", publishedStatus, published)
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload, status, attempts, last_error)
		VALUES (gen_random_uuid(), 'parcel.assigned', 'parcel.assigned', '{}'::jsonb, 'FAILED', 3, 'broker unavailable')`); err != nil {
		t.Fatalf("insert failed event: %v", err)
	}
	var failedAttempts int
	var lastError string
	if err := conn.QueryRow(ctx, `
		SELECT attempts, last_error FROM parcel.outbox_events
		WHERE status = 'FAILED'`).Scan(&failedAttempts, &lastError); err != nil {
		t.Fatalf("read failed event: %v", err)
	}
	if failedAttempts != 3 || lastError != "broker unavailable" {
		t.Errorf("failed event = attempts %d error %q; want 3 / broker unavailable", failedAttempts, lastError)
	}

	_, duplicateErr := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
		VALUES ($1, 'parcel.created', 'parcel.created', '{}'::jsonb)`, eventID)
	assertSQLState(t, duplicateErr, "23505")

	_, pairErr := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
		VALUES (gen_random_uuid(), 'parcel.status_changed', 'parcel.status_changed', '{}'::jsonb)`)
	assertSQLState(t, pairErr, "23514")

	_, aggregatePairErr := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload, aggregate_type)
		VALUES (gen_random_uuid(), 'parcel.created', 'parcel.created', '{}'::jsonb, 'parcel')`)
	assertSQLState(t, aggregatePairErr, "23514")

	if _, err := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
		VALUES (gen_random_uuid(), 'parcel.created', 'parcel.created', '{}'::jsonb)`); err != nil {
		t.Fatalf("insert event with null aggregate pair: %v", err)
	}
}

func TestOutboxMigration000005(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 5)

	parcelID := insertParcel(t, ctx, conn, "PICKED_UP")

	validEvents := []struct {
		eventType  string
		routingKey string
	}{
		{"parcel.created", "parcel.created"},
		{"parcel.assigned", "parcel.assigned"},
		{"parcel.status_changed", "parcel.status.changed"},
	}

	for _, tc := range validEvents {
		t.Run(tc.eventType+"_"+tc.routingKey, func(t *testing.T) {
			eventID := newUUID(t, ctx, conn)
			_, err := conn.Exec(ctx, `
				INSERT INTO parcel.outbox_events (
					id, event_type, routing_key, payload, aggregate_type, aggregate_id
				) VALUES ($1, $2, $3, '{}'::jsonb, 'parcel', $4)`,
				eventID, tc.eventType, tc.routingKey, parcelID)
			if err != nil {
				t.Fatalf("insert valid outbox event (%s, %s): %v", tc.eventType, tc.routingKey, err)
			}
		})
	}

	invalidEvents := []struct {
		name       string
		eventType  string
		routingKey string
	}{
		{"status_changed_underscore_key", "parcel.status_changed", "parcel.status_changed"},
		{"status_changed_to_created", "parcel.status_changed", "parcel.created"},
		{"created_to_status_changed", "parcel.created", "parcel.status.changed"},
		{"created_to_assigned", "parcel.created", "parcel.assigned"},
		{"versioned_created", "parcel.created.v1", "parcel.created.v1"},
		{"versioned_status_changed", "parcel.status_changed.v1", "parcel.status_changed.v1"},
		{"mixed_version", "parcel.created.v1", "parcel.created"},
		{"dot_event_type", "parcel.status.changed", "parcel.status.changed"},
		{"unsupported_event_type", "parcel.deleted", "parcel.deleted"},
		{"unsupported_routing_key", "parcel.created", "parcel.deleted"},
	}

	for _, tc := range invalidEvents {
		t.Run(tc.name, func(t *testing.T) {
			eventID := newUUID(t, ctx, conn)
			_, err := conn.Exec(ctx, `
				INSERT INTO parcel.outbox_events (
					id, event_type, routing_key, payload
				) VALUES ($1, $2, $3, '{}'::jsonb)`,
				eventID, tc.eventType, tc.routingKey)
			assertSQLState(t, err, "23514")
		})
	}

	// Final column definitions after 000005.
	for column, want := range map[string]struct {
		dataType string
		nullable string
	}{
		"event_type":  {"character varying", "NO"},
		"routing_key": {"character varying", "NO"},
		"last_error":  {"text", "YES"},
	} {
		dataType, nullable := columnType(t, ctx, conn, "outbox_events", column)
		if dataType != want.dataType || nullable != want.nullable {
			t.Errorf("outbox_events.%s = (%s, nullable=%s); want (%s, nullable=%s)",
				column, dataType, nullable, want.dataType, want.nullable)
		}
	}

	for _, constraint := range []string{
		"outbox_events_event_type_check",
		"outbox_events_routing_key_check",
		"outbox_type_key_pair",
	} {
		if !constraintExists(t, ctx, conn, "outbox_events", constraint) {
			t.Errorf("missing constraint %s on parcel.outbox_events", constraint)
		}
	}

	// last_error is TEXT, so a long diagnostic is stored without truncation.
	longError := strings.Repeat("broker unavailable: ", 60)
	var longErrorID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload, status, attempts, last_error)
		VALUES (gen_random_uuid(), 'parcel.created', 'parcel.created', '{}'::jsonb, 'FAILED', 1, $1)
		RETURNING id::text`, longError).Scan(&longErrorID); err != nil {
		t.Fatalf("insert long last_error: %v", err)
	}
	var storedLength int
	if err := conn.QueryRow(ctx, `SELECT length(last_error) FROM parcel.outbox_events WHERE id = $1`, longErrorID).
		Scan(&storedLength); err != nil {
		t.Fatalf("read long last_error: %v", err)
	}
	if storedLength != len(longError) {
		t.Errorf("last_error length = %d; want %d (no truncation)", storedLength, len(longError))
	}

	// The pending FIFO poll is served by the baseline composite index; 000005
	// intentionally does not add a duplicate partial index.
	if !indexExists(t, ctx, conn, "outbox_events", "outbox_status_created") {
		t.Error("outbox_status_created index is missing")
	}
	if indexExists(t, ctx, conn, "outbox_events", "idx_outbox_events_pending_relay") {
		t.Error("idx_outbox_events_pending_relay duplicates outbox_status_created")
	}

	columns, predicate := indexDefinition(t, ctx, conn, "idx_outbox_events_published_cleanup")
	if strings.Join(columns, ",") != "published_at" {
		t.Errorf("idx_outbox_events_published_cleanup columns = %v; want [published_at]", columns)
	}
	if !strings.Contains(predicate, "status") || !strings.Contains(predicate, "PUBLISHED") {
		t.Errorf("idx_outbox_events_published_cleanup predicate = %q; want status = 'PUBLISHED'", predicate)
	}

	columns, predicate = indexDefinition(t, ctx, conn, "idx_parcel_assignments_courier_workload")
	if strings.Join(columns, ",") != "courier_id" {
		t.Errorf("idx_parcel_assignments_courier_workload columns = %v; want [courier_id]", columns)
	}
	if !strings.Contains(predicate, "is_current") {
		t.Errorf("idx_parcel_assignments_courier_workload predicate = %q; want is_current", predicate)
	}
}

func TestOutboxMigration000005RollbackPrecondition(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 5)

	longError := strings.Repeat("diagnostic: ", 40)
	if _, err := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload, status, attempts, last_error)
		VALUES (gen_random_uuid(), 'parcel.created', 'parcel.created', '{}'::jsonb, 'FAILED', 1, $1)`,
		longError); err != nil {
		t.Fatalf("insert long last_error: %v", err)
	}

	err := execSQLFile(ctx, conn, "migrations/parcel/000005_outbox_event_types.down.sql")
	if err == nil {
		t.Fatal("rollback succeeded with an over-length last_error; want a clear failure")
	}
	if !strings.Contains(err.Error(), "longer than 255 characters") {
		t.Errorf("rollback error = %v; want a clear last_error length precondition", err)
	}

	// The failed rollback aborts its transaction; clear it before inspecting.
	if _, rollbackErr := conn.Exec(ctx, "ROLLBACK"); rollbackErr != nil {
		t.Logf("clear aborted rollback transaction: %v", rollbackErr)
	}

	// The failed rollback must not leave a partially modified schema.
	if dataType, _ := columnType(t, ctx, conn, "outbox_events", "last_error"); dataType != "text" {
		t.Errorf("last_error type after failed rollback = %s; want text", dataType)
	}
	if !indexExists(t, ctx, conn, "outbox_events", "idx_outbox_events_published_cleanup") {
		t.Error("published cleanup index disappeared after failed rollback")
	}
	if !indexExists(t, ctx, conn, "parcel_assignments", "idx_parcel_assignments_courier_workload") {
		t.Error("workload index disappeared after failed rollback")
	}

	// Once the over-length diagnostic is shortened, rollback succeeds.
	if _, err := conn.Exec(ctx, `UPDATE parcel.outbox_events SET last_error = 'short'`); err != nil {
		t.Fatalf("shorten last_error: %v", err)
	}
	if err := execSQLFile(ctx, conn, "migrations/parcel/000005_outbox_event_types.down.sql"); err != nil {
		t.Fatalf("rollback after shortening last_error: %v", err)
	}
	if dataType, _ := columnType(t, ctx, conn, "outbox_events", "last_error"); dataType != "character varying" {
		t.Errorf("last_error type after rollback = %s; want character varying", dataType)
	}
	if indexExists(t, ctx, conn, "outbox_events", "idx_outbox_events_published_cleanup") {
		t.Error("published cleanup index still exists after rollback")
	}
	if indexExists(t, ctx, conn, "parcel_assignments", "idx_parcel_assignments_courier_workload") {
		t.Error("workload index still exists after rollback")
	}
	if !constraintExists(t, ctx, conn, "outbox_events", "outbox_type_key_pair") {
		t.Error("outbox_type_key_pair missing after rollback")
	}

	_, invalidErr := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
		VALUES (gen_random_uuid(), 'parcel.status_changed', 'parcel.status_changed', '{}'::jsonb)`)
	assertSQLState(t, invalidErr, "23514")

	if _, err := conn.Exec(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
		VALUES (gen_random_uuid(), 'parcel.status_changed', 'parcel.status.changed', '{}'::jsonb)`); err != nil {
		t.Fatalf("insert valid pair after rollback: %v", err)
	}
}

func TestParcelMigrationUpgradeFromBaseline(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, 2)

	parcelID := insertParcel(t, ctx, conn, "CREATED")
	courierID := newUUID(t, ctx, conn)
	assignmentID := insertBaselineAssignment(t, ctx, conn, parcelID, courierID, true)

	var outboxID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
		VALUES (gen_random_uuid(), 'parcel.created', 'parcel.created', '{}'::jsonb)
		RETURNING id::text`).Scan(&outboxID); err != nil {
		t.Fatalf("insert baseline outbox row: %v", err)
	}

	applySQLFile(t, ctx, conn, "migrations/parcel/000003_dispatch_foundations.up.sql")

	var previousCourier *string
	var updatedAt time.Time
	if err := conn.QueryRow(ctx, `
		SELECT previous_courier_id, updated_at FROM parcel.parcel_assignments WHERE id = $1`, assignmentID).
		Scan(&previousCourier, &updatedAt); err != nil {
		t.Fatalf("read upgraded assignment: %v", err)
	}
	if previousCourier != nil || updatedAt.IsZero() {
		t.Errorf("upgraded assignment = previous %v updated_at %v; want nil and a timestamp", previousCourier, updatedAt)
	}

	var aggregateType *string
	var aggregateID *string
	if err := conn.QueryRow(ctx, `
		SELECT aggregate_type, aggregate_id::text FROM parcel.outbox_events WHERE id = $1`, outboxID).
		Scan(&aggregateType, &aggregateID); err != nil {
		t.Fatalf("read upgraded outbox row: %v", err)
	}
	if aggregateType != nil || aggregateID != nil {
		t.Errorf("upgraded outbox aggregate = %v/%v; want nil/nil", aggregateType, aggregateID)
	}

	applySQLFile(t, ctx, conn, "migrations/parcel/000003_dispatch_foundations.down.sql")

	if columnExists(t, ctx, conn, "parcel_assignments", "previous_courier_id") {
		t.Error("previous_courier_id still exists after rollback")
	}
	if columnExists(t, ctx, conn, "outbox_events", "aggregate_type") {
		t.Error("aggregate_type still exists after rollback")
	}
	var assignmentsAfterRollback, outboxAfterRollback int
	if err := conn.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM parcel.parcel_assignments),
			(SELECT count(*) FROM parcel.outbox_events)`).
		Scan(&assignmentsAfterRollback, &outboxAfterRollback); err != nil {
		t.Fatalf("count rows after rollback: %v", err)
	}
	if assignmentsAfterRollback != 1 || outboxAfterRollback != 1 {
		t.Errorf("rows after rollback = %d assignments / %d outbox; want 1/1", assignmentsAfterRollback, outboxAfterRollback)
	}

	applySQLFile(t, ctx, conn, "migrations/parcel/000003_dispatch_foundations.up.sql")
	if !columnExists(t, ctx, conn, "parcel_assignments", "previous_courier_id") {
		t.Error("previous_courier_id missing after reapply")
	}
	if !columnExists(t, ctx, conn, "outbox_events", "aggregate_id") {
		t.Error("aggregate_id missing after reapply")
	}
}

func TestParcelMigrationsFullRollbackAndReapply(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyParcelMigrations(t, ctx, conn, len(parcelUpMigrations))

	if !tableExists(t, ctx, conn, "parcel_assignments") || !tableExists(t, ctx, conn, "outbox_events") {
		t.Fatal("baseline tables missing after full migration")
	}
	if dataType, _ := columnType(t, ctx, conn, "outbox_events", "last_error"); dataType != "text" {
		t.Fatalf("last_error type after full migration = %s; want text", dataType)
	}

	rollbackParcelMigrations(t, ctx, conn, len(parcelDownMigrations))

	if schemaExists(t, ctx, conn, "parcel") {
		t.Fatal("parcel schema still exists after full rollback")
	}

	applyParcelMigrations(t, ctx, conn, len(parcelUpMigrations))

	if !tableExists(t, ctx, conn, "parcel_assignments") || !tableExists(t, ctx, conn, "outbox_events") {
		t.Fatal("baseline tables missing after reapply")
	}
	if dataType, _ := columnType(t, ctx, conn, "outbox_events", "last_error"); dataType != "text" {
		t.Errorf("last_error type after reapply = %s; want text", dataType)
	}
	if !indexExists(t, ctx, conn, "outbox_events", "idx_outbox_events_published_cleanup") {
		t.Error("published cleanup index missing after reapply")
	}
	if !indexExists(t, ctx, conn, "parcel_assignments", "idx_parcel_assignments_courier_workload") {
		t.Error("workload index missing after reapply")
	}
	if !constraintExists(t, ctx, conn, "outbox_events", "outbox_type_key_pair") {
		t.Error("outbox_type_key_pair missing after reapply")
	}

	parcelID := insertParcel(t, ctx, conn, "CREATED")
	insertAssignment(t, ctx, conn, parcelID, newUUID(t, ctx, conn), nil, true)
}

func TestMigrationVersionFiles(t *testing.T) {
	files, err := filepath.Glob(repositoryPath("migrations", "*", "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}

	namePattern := regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.(up|down)\.sql$`)
	type versionFiles struct {
		up   string
		down string
	}
	services := map[string]map[string]*versionFiles{}

	for _, file := range files {
		base := filepath.Base(file)
		service := filepath.Base(filepath.Dir(file))
		matches := namePattern.FindStringSubmatch(base)
		if matches == nil {
			t.Errorf("migration filename does not match convention: %s", file)
			continue
		}
		version, kind := matches[1], matches[3]
		if services[service] == nil {
			services[service] = map[string]*versionFiles{}
		}
		if services[service][version] == nil {
			services[service][version] = &versionFiles{}
		}
		if kind == "up" {
			services[service][version].up = file
		} else {
			services[service][version].down = file
		}
	}

	for service, versions := range services {
		for version, pair := range versions {
			if pair.up == "" || pair.down == "" {
				t.Errorf("%s version %s is missing an up or down file", service, version)
			}
		}
	}

	// Each service has its own migration version table, so versions are unique
	// per service directory rather than across the whole repository.
	if len(services["parcel"]) != 7 {
		t.Errorf("parcel migration version count = %d; want 7", len(services["parcel"]))
	}
	for _, version := range []string{"000001", "000002", "000003", "000004", "000005", "000006", "000007"} {
		if services["parcel"][version] == nil {
			t.Errorf("parcel migration %s is missing", version)
		}
	}

	baseline, err := os.ReadFile(repositoryPath("migrations/parcel/000002_tables.up.sql"))
	if err != nil {
		t.Fatalf("read baseline migration: %v", err)
	}
	if !strings.Contains(string(baseline), "CREATE TABLE IF NOT EXISTS parcel.parcel_assignments") {
		t.Error("baseline migration does not create parcel_assignments")
	}
	if !strings.Contains(string(baseline), "CREATE TABLE IF NOT EXISTS parcel.outbox_events") {
		t.Error("baseline migration does not create outbox_events")
	}

	foundation, err := os.ReadFile(repositoryPath("migrations/parcel/000003_dispatch_foundations.up.sql"))
	if err != nil {
		t.Fatalf("read dispatch migration: %v", err)
	}
	if strings.Contains(string(foundation), "CREATE TABLE") {
		t.Error("000003_dispatch_foundations must not create tables")
	}
	for _, object := range []string{"previous_courier_id", "aggregate_type", "aggregate_id"} {
		if !strings.Contains(string(foundation), object) {
			t.Errorf("000003_dispatch_foundations does not add %s", object)
		}
	}

	outboxFix, err := os.ReadFile(repositoryPath("migrations/parcel/000005_outbox_event_types.up.sql"))
	if err != nil {
		t.Fatalf("read outbox fix migration: %v", err)
	}
	normalizedOutboxFix := strings.Join(strings.Fields(string(outboxFix)), " ")
	for _, want := range []string{
		"event_type IN ('parcel.created', 'parcel.assigned', 'parcel.status_changed')",
		"routing_key IN ('parcel.created', 'parcel.assigned', 'parcel.status.changed')",
		"(event_type = 'parcel.status_changed' AND routing_key = 'parcel.status.changed')",
		"idx_outbox_events_published_cleanup",
		"idx_parcel_assignments_courier_workload",
	} {
		if !strings.Contains(normalizedOutboxFix, want) {
			t.Errorf("000005_outbox_event_types does not contain %q", want)
		}
	}
	if strings.Contains(normalizedOutboxFix, "idx_outbox_events_pending_relay") {
		t.Error("000005_outbox_event_types must not add a duplicate pending-relay index")
	}

	outboxClaim, err := os.ReadFile(repositoryPath("migrations/parcel/000007_outbox_claim.up.sql"))
	if err != nil {
		t.Fatalf("read outbox claim migration: %v", err)
	}
	if strings.Contains(string(outboxClaim), "CREATE TABLE") {
		t.Error("000007_outbox_claim must not create tables")
	}
	for _, column := range []string{"claim_token UUID", "claimed_at TIMESTAMPTZ"} {
		if !strings.Contains(string(outboxClaim), column) {
			t.Errorf("000007_outbox_claim does not add %s", column)
		}
	}

	var versions []string
	for version := range services["parcel"] {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	if strings.Join(versions, ",") != "000001,000002,000003,000004,000005,000006,000007" {
		t.Errorf("parcel versions = %v; want [000001 000002 000003 000004 000005 000006 000007]", versions)
	}
}

func columnExists(t *testing.T, ctx context.Context, conn *pgx.Conn, table, column string) bool {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'parcel' AND table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists); err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	return exists
}

func tableExists(t *testing.T, ctx context.Context, conn *pgx.Conn, table string) bool {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT to_regclass('parcel.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	return exists
}
