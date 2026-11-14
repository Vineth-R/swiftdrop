package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

type fakeRows struct {
	nextFn func() bool
	scanFn func(dest ...any) error
	errFn  func() error
}

func (r *fakeRows) Close() {}
func (r *fakeRows) Err() error {
	if r.errFn != nil {
		return r.errFn()
	}
	return nil
}
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool {
	if r.nextFn != nil {
		return r.nextFn()
	}
	return false
}
func (r *fakeRows) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}
func (r *fakeRows) Values() ([]any, error) { return nil, nil }
func (r *fakeRows) RawValues() [][]byte    { return nil }
func (r *fakeRows) Conn() *pgx.Conn        { return nil }
func (r *fakeRows) TypeMap() *pgtype.Map   { return pgtype.NewMap() }

type fakeTx struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	commitFn   func(ctx context.Context) error
	rollbackFn func(ctx context.Context) error
}

func (tx *fakeTx) Begin(ctx context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *fakeTx) Commit(ctx context.Context) error {
	if tx.commitFn != nil {
		return tx.commitFn(ctx)
	}
	return nil
}
func (tx *fakeTx) Rollback(ctx context.Context) error {
	if tx.rollbackFn != nil {
		return tx.rollbackFn(ctx)
	}
	return nil
}
func (tx *fakeTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (tx *fakeTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (tx *fakeTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (tx *fakeTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (tx *fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx.execFn != nil {
		return tx.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}
func (tx *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx.queryFn != nil {
		return tx.queryFn(ctx, sql, args...)
	}
	return &fakeRows{}, nil
}
func (tx *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx.queryRowFn != nil {
		return tx.queryRowFn(ctx, sql, args...)
	}
	return &fakeRow{}
}
func (tx *fakeTx) Conn() *pgx.Conn { return nil }

func TestEscapeLike(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"SDK12345678", "SDK12345678"},
		{"SDK_123", `SDK\_123`},
		{"SDK%123", `SDK\%123`},
		{`SDK\123`, `SDK\\123`},
		{`%_\`, `\%\_\\`},
	}

	for _, c := range cases {
		if got := escapeLike(c.input); got != c.want {
			t.Errorf("escapeLike(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

type mockDB struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	beginFn    func(ctx context.Context) (pgx.Tx, error)
}

func (db *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if db.execFn != nil {
		return db.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (db *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFn != nil {
		return db.queryRowFn(ctx, sql, args...)
	}
	return &fakeRow{}
}

func (db *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if db.queryFn != nil {
		return db.queryFn(ctx, sql, args...)
	}
	return &fakeRows{}, nil
}

func (db *mockDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if db.beginFn != nil {
		return db.beginFn(ctx)
	}
	return &fakeTx{}, nil
}

func TestRepositoryNilDB(t *testing.T) {
	repo := NewRepository(nil)
	ctx := context.Background()

	if _, err := repo.ListUnassigned(ctx, UnassignedFilter{}); err == nil {
		t.Error("expected error for nil DB in ListUnassigned")
	}
	if _, err := repo.AssignCourier(ctx, AssignInput{}); err == nil {
		t.Error("expected error for nil DB in AssignCourier")
	}
	if _, err := repo.GetActiveWorkloadCounts(ctx); err == nil {
		t.Error("expected error for nil DB in GetActiveWorkloadCounts")
	}
	if _, err := repo.ListAdminParcels(ctx, AdminParcelFilter{}); err == nil {
		t.Error("expected error for nil DB in ListAdminParcels")
	}
}

func TestRepositoryListUnassigned(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	zoneID := "3f1c7a52-0000-4000-8000-000000000001"

	var rowCount int
	db := &mockDB{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &fakeRow{
				scanFn: func(dest ...any) error {
					*dest[0].(*int) = 1
					return nil
				},
			}
		},
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			rowCount = 0
			return &fakeRows{
				nextFn: func() bool {
					rowCount++
					return rowCount <= 1
				},
				scanFn: func(dest ...any) error {
					*dest[0].(*string) = "parcel-1"
					*dest[1].(*string) = "SDK12345678"
					*dest[2].(*string) = "CREATED"
					*dest[3].(*string) = "1 Pickup St"
					*dest[4].(*string) = "Colombo"
					*dest[5].(**string) = &zoneID
					*dest[6].(*string) = "1 Dropoff St"
					*dest[7].(*string) = "Kandy"
					dropoffZone := "3f1c7a52-0000-4000-8000-000000000002"
					*dest[8].(**string) = &dropoffZone
					*dest[9].(*pgtype.Numeric) = pgtype.Numeric{Valid: true}
					*dest[10].(*string) = "SMALL"
					*dest[11].(*time.Time) = now
					return nil
				},
			}, nil
		},
	}

	repo := NewRepository(db)
	page, err := repo.ListUnassigned(ctx, UnassignedFilter{
		ZoneID:   &zoneID,
		From:     &now,
		To:       &now,
		Page:     1,
		PageSize: 10,
		Sort:     "createdAt:desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 {
		t.Errorf("expected 1 item, got %d (total %d)", len(page.Items), page.TotalItems)
	}
}

func TestRepositoryAssignCourier(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	parcelID := "3f1c7a52-0000-4000-8000-000000000001"
	courierID := "3f1c7a52-0000-4000-8000-000000000002"
	actorID := "3f1c7a52-0000-4000-8000-000000000003"
	prevCourierID := "3f1c7a52-0000-4000-8000-000000000004"

	t.Run("parcel not found", func(t *testing.T) {
		tx := &fakeTx{
			queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
				return &fakeRow{
					scanFn: func(dest ...any) error {
						return pgx.ErrNoRows
					},
				}
			},
		}
		repo := NewRepository(&mockDB{
			beginFn: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
		})
		_, err := repo.AssignCourier(ctx, AssignInput{ParcelID: parcelID, CourierID: courierID})
		if !errors.Is(err, ErrParcelNotFound) {
			t.Errorf("expected ErrParcelNotFound, got %v", err)
		}
	})

	t.Run("parcel closed", func(t *testing.T) {
		tx := &fakeTx{
			queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
				return &fakeRow{
					scanFn: func(dest ...any) error {
						*dest[0].(*string) = "SDK12345678"
						*dest[1].(*string) = "DELIVERED"
						return nil
					},
				}
			},
		}
		repo := NewRepository(&mockDB{
			beginFn: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
		})
		_, err := repo.AssignCourier(ctx, AssignInput{ParcelID: parcelID, CourierID: courierID})
		if !errors.Is(err, ErrParcelClosed) {
			t.Errorf("expected ErrParcelClosed, got %v", err)
		}
	})

	t.Run("successful assignment with reassignment", func(t *testing.T) {
		queryCount := 0
		tx := &fakeTx{
			queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
				queryCount++
				return &fakeRow{
					scanFn: func(dest ...any) error {
						switch queryCount {
						case 1:
							// SELECT tracking_number, status FROM parcel.parcels FOR UPDATE
							*dest[0].(*string) = "SDK12345678"
							*dest[1].(*string) = "PICKED_UP"
						case 2:
							// SELECT id, courier_id FROM parcel.parcel_assignments WHERE parcel_id = $1 AND is_current = true
							*dest[0].(*string) = "assignment-1"
							prev := prevCourierID
							*dest[1].(**string) = &prev
						case 3:
							// INSERT INTO parcel.parcel_assignments ... RETURNING assigned_at
							*dest[0].(*time.Time) = now
						}
						return nil
					},
				}
			},
		}
		repo := NewRepository(&mockDB{
			beginFn: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
		})
		res, err := repo.AssignCourier(ctx, AssignInput{
			ParcelID:    parcelID,
			CourierID:   courierID,
			ActorUserID: actorID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ParcelID != parcelID || res.PreviousCourierID == nil || *res.PreviousCourierID != prevCourierID {
			t.Errorf("unexpected assign result: %+v", res)
		}
	})
}

func TestRepositoryWorkloadCounts(t *testing.T) {
	ctx := context.Background()
	var rowCount int

	db := &mockDB{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			rowCount = 0
			return &fakeRows{
				nextFn: func() bool {
					rowCount++
					return rowCount <= 2
				},
				scanFn: func(dest ...any) error {
					if rowCount == 1 {
						*dest[0].(*string) = "c1"
						*dest[1].(*string) = "CREATED"
						*dest[2].(*int) = 3
					} else {
						*dest[0].(*string) = "c1"
						*dest[1].(*string) = "PICKED_UP"
						*dest[2].(*int) = 2
					}
					return nil
				},
			}, nil
		},
	}

	repo := NewRepository(db)
	counts, err := repo.GetActiveWorkloadCounts(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c1Counts, ok := counts["c1"]
	if !ok || c1Counts.Total != 5 || c1Counts.ByStatus["CREATED"] != 3 || c1Counts.ByStatus["PICKED_UP"] != 2 {
		t.Errorf("unexpected counts: %+v", counts)
	}
}

func TestRepositoryListAdminParcels(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	status := "CREATED"
	courierID := "3f1c7a52-0000-4000-8000-000000000001"
	merchantID := "3f1c7a52-0000-4000-8000-000000000002"
	customerID := "3f1c7a52-0000-4000-8000-000000000003"
	tracking := "SDK12"

	var rowCount int
	db := &mockDB{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &fakeRow{
				scanFn: func(dest ...any) error {
					*dest[0].(*int) = 1
					return nil
				},
			}
		},
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			rowCount = 0
			return &fakeRows{
				nextFn: func() bool {
					rowCount++
					return rowCount <= 1
				},
				scanFn: func(dest ...any) error {
					*dest[0].(*string) = "parcel-1"
					*dest[1].(*string) = "SDK12345678"
					*dest[2].(*string) = "CREATED"
					*dest[3].(*string) = "Colombo"
					*dest[4].(*string) = "Kandy"
					*dest[5].(**string) = nil
					*dest[6].(**string) = nil
					cust := customerID
					*dest[7].(**string) = &cust
					*dest[8].(*pgtype.Numeric) = pgtype.Numeric{Valid: true}
					*dest[9].(*time.Time) = now
					return nil
				},
			}, nil
		},
	}

	repo := NewRepository(db)
	page, err := repo.ListAdminParcels(ctx, AdminParcelFilter{
		Status:         &status,
		CourierID:      &courierID,
		MerchantID:     &merchantID,
		CustomerID:     &customerID,
		TrackingNumber: &tracking,
		From:           &now,
		To:             &now,
		Page:           1,
		PageSize:       20,
		Sort:           "createdAt:asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.TotalItems != 1 {
		t.Errorf("expected 1 item, got %d", page.TotalItems)
	}
}

func TestRepositoryAssignCourierClosedStatuses(t *testing.T) {
	ctx := context.Background()
	parcelID := "3f1c7a52-0000-4000-8000-000000000001"
	courierID := "3f1c7a52-0000-4000-8000-000000000002"

	for _, closedStatus := range []string{"DELIVERED", "FAILED"} {
		t.Run(closedStatus, func(t *testing.T) {
			var execCount int
			db := &mockDB{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return &fakeTx{
						queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
							return &fakeRow{
								scanFn: func(dest ...any) error {
									*dest[0].(*string) = "SDK12345678"
									*dest[1].(*string) = closedStatus
									return nil
								},
							}
						},
						execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
							execCount++
							return pgconn.CommandTag{}, nil
						},
					}, nil
				},
			}

			repo := NewRepository(db)
			_, err := repo.AssignCourier(ctx, AssignInput{
				ParcelID:    parcelID,
				CourierID:   courierID,
				ActorUserID: "3f1c7a52-0000-4000-8000-000000000003",
				Reason:      "test assignment on closed parcel",
			})

			if !errors.Is(err, ErrParcelClosed) {
				t.Fatalf("status %s: expected ErrParcelClosed, got: %v", closedStatus, err)
			}
			if execCount != 0 {
				t.Errorf("expected 0 write executions on closed parcel, got %d", execCount)
			}
		})
	}
}

func TestConcurrentAssignCourierSingleActive(t *testing.T) {
	ctx := context.Background()
	parcelID := "3f1c7a52-0000-4000-8000-000000000001"

	type assignmentRecord struct {
		id                string
		parcelID          string
		courierID         string
		previousCourierID *string
		isCurrent         bool
		unassignedAt      *time.Time
	}

	type simulatedDBState struct {
		mu               sync.Mutex
		currentCourierID *string
		status           string
		assignments      []assignmentRecord
		outboxEvents     []string
	}

	state := &simulatedDBState{
		status: "CREATED",
	}

	concurrentWorkers := 20
	var wg sync.WaitGroup
	start := make(chan struct{})

	safeDB := &mockDB{
		beginFn: func(ctx context.Context) (pgx.Tx, error) {
			state.mu.Lock()
			txCommitted := false

			queryStep := 0
			var activeAssignmentID string
			var activeCourierID *string

			tx := &fakeTx{
				queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
					queryStep++
					if queryStep == 1 {
						return &fakeRow{
							scanFn: func(dest ...any) error {
								*dest[0].(*string) = "SDK12345678"
								*dest[1].(*string) = state.status
								return nil
							},
						}
					}
					if queryStep == 2 {
						activeAssignmentID = ""
						activeCourierID = nil
						for i := range state.assignments {
							if state.assignments[i].parcelID == parcelID && state.assignments[i].isCurrent {
								activeAssignmentID = state.assignments[i].id
								cID := state.assignments[i].courierID
								activeCourierID = &cID
								break
							}
						}
						return &fakeRow{
							scanFn: func(dest ...any) error {
								if activeAssignmentID == "" {
									return pgx.ErrNoRows
								}
								*dest[0].(*string) = activeAssignmentID
								*dest[1].(**string) = activeCourierID
								return nil
							},
						}
					}
					if queryStep == 3 {
						newCourierID := args[1].(string)
						var prevCID *string
						if args[2] != nil {
							prevCID = args[2].(*string)
						}
						newID := fmt.Sprintf("assign-%d", len(state.assignments)+1)
						state.assignments = append(state.assignments, assignmentRecord{
							id:                newID,
							parcelID:          parcelID,
							courierID:         newCourierID,
							previousCourierID: prevCID,
							isCurrent:         true,
						})
						return &fakeRow{
							scanFn: func(dest ...any) error {
								*dest[0].(*time.Time) = time.Now().UTC()
								return nil
							},
						}
					}
					return &fakeRow{}
				},
				execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					if strings.Contains(sql, "parcel.parcel_assignments") && activeAssignmentID != "" {
						for i := range state.assignments {
							if state.assignments[i].id == activeAssignmentID {
								state.assignments[i].isCurrent = false
								now := time.Now().UTC()
								state.assignments[i].unassignedAt = &now
							}
						}
					}
					if strings.Contains(sql, "UPDATE parcel.parcels") {
						courierID := args[1].(string)
						state.currentCourierID = &courierID
					}
					if strings.Contains(sql, "INSERT INTO parcel.outbox_events") {
						state.outboxEvents = append(state.outboxEvents, "parcel.assigned")
					}
					return pgconn.CommandTag{}, nil
				},
				commitFn: func(ctx context.Context) error {
					txCommitted = true
					state.mu.Unlock()
					return nil
				},
				rollbackFn: func(ctx context.Context) error {
					if !txCommitted {
						state.mu.Unlock()
					}
					return nil
				},
			}
			return tx, nil
		},
	}

	repo := NewRepository(safeDB)

	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		courierUUID := fmt.Sprintf("3f1c7a52-0000-4000-8000-%012d", i+1)
		go func(cid string) {
			defer wg.Done()
			<-start

			res, err := repo.AssignCourier(ctx, AssignInput{
				ParcelID:    parcelID,
				CourierID:   cid,
				ActorUserID: "3f1c7a52-0000-4000-8000-000000000099",
				Reason:      "concurrent reassignment test",
			})
			if err != nil {
				t.Errorf("AssignCourier worker %s failed: %v", cid, err)
			}
			if res.ParcelID != parcelID {
				t.Errorf("worker %s returned unexpected ParcelID %s", cid, res.ParcelID)
			}
		}(courierUUID)
	}

	close(start)
	wg.Wait()

	state.mu.Lock()
	defer state.mu.Unlock()

	var currentCount int
	var currentAssignment *assignmentRecord
	for i := range state.assignments {
		if state.assignments[i].isCurrent {
			currentCount++
			currentAssignment = &state.assignments[i]
		}
	}

	if currentCount != 1 {
		t.Fatalf("expected exactly 1 current assignment, found %d", currentCount)
	}
	if currentAssignment.unassignedAt != nil {
		t.Fatalf("active assignment must have unassigned_at == nil, got %v", currentAssignment.unassignedAt)
	}
	for i := range state.assignments {
		if !state.assignments[i].isCurrent && state.assignments[i].unassignedAt == nil {
			t.Errorf("closed assignment %s must have non-nil unassigned_at", state.assignments[i].id)
		}
	}
	if state.currentCourierID == nil || *state.currentCourierID != currentAssignment.courierID {
		t.Fatalf("parcels.current_courier_id (%v) does not match active assignment courier (%s)",
			state.currentCourierID, currentAssignment.courierID)
	}
	if len(state.outboxEvents) != concurrentWorkers {
		t.Fatalf("expected %d outbox events, found %d", concurrentWorkers, len(state.outboxEvents))
	}
	if len(state.assignments) != concurrentWorkers {
		t.Fatalf("expected %d total assignment history rows, found %d", concurrentWorkers, len(state.assignments))
	}
}
