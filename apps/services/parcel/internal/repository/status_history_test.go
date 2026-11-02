package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAppendStatusHistoryPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	database := &fakeDB{row: errorRow{err: context.Canceled}}
	repository, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if _, err := repository.AppendStatusHistory(ctx, AppendStatusHistoryParams{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("AppendStatusHistory() error = %v, want context.Canceled", err)
	}
	if database.context != ctx {
		t.Fatal("AppendStatusHistory() did not propagate its context")
	}
}

func TestStatusHistoryByParcelClosesRows(t *testing.T) {
	scanError := errors.New("scan failed")
	iterationError := errors.New("connection lost while reading")
	tests := []struct {
		name string
		rows fakeRows
		want error
	}{
		{name: "empty history"},
		{name: "scan failure", rows: fakeRows{remaining: 1, scanErr: scanError}, want: scanError},
		{name: "iteration failure", rows: fakeRows{iterationErr: iterationError}, want: iterationError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := &fakeDB{rows: &test.rows}
			repository, err := New(database)
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			history, err := repository.StatusHistoryByParcel(context.Background(), "parcel")
			if !errors.Is(err, test.want) {
				t.Fatalf("StatusHistoryByParcel() error = %v, want %v", err, test.want)
			}
			if !test.rows.closed {
				t.Fatal("StatusHistoryByParcel() did not close rows")
			}
			if test.want == nil && (history == nil || len(history) != 0) {
				t.Fatal("empty history should return an initialized empty slice")
			}
		})
	}
}

func TestStatusHistoryByParcelPropagatesQueryFailure(t *testing.T) {
	database := &fakeDB{queryErr: context.DeadlineExceeded}
	repository, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := repository.StatusHistoryByParcel(ctx, "parcel"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StatusHistoryByParcel() error = %v, want context.DeadlineExceeded", err)
	}
	if database.context != ctx {
		t.Fatal("StatusHistoryByParcel() did not propagate its context")
	}
}

type fakeRows struct {
	pgx.Rows
	remaining    int
	scanErr      error
	iterationErr error
	closed       bool
}

func (rows *fakeRows) Next() bool {
	if rows.remaining == 0 {
		return false
	}
	rows.remaining--
	return true
}

func (rows *fakeRows) Scan(_ ...any) error {
	return rows.scanErr
}

func (rows *fakeRows) Err() error {
	return rows.iterationErr
}

func (rows *fakeRows) Close() {
	rows.closed = true
}
