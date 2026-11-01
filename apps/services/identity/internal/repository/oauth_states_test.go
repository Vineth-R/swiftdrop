package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeDB answers every query with a canned row or error. It lets the Go side
// of a repository method be tested without PostgreSQL; the SQL itself is
// covered by the integration tests.
type fakeDB struct {
	row     pgx.Row
	execErr error

	lastSQL  string
	lastArgs []any
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.lastSQL, f.lastArgs = sql, args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.lastSQL, f.lastArgs = sql, args
	return f.row
}

func (f *fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("fakeDB: Query is not supported")
}

func (f *fakeDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("fakeDB: Begin is not supported")
}

// fakeRow copies the given values into Scan destinations in order. Pointer
// destinations of type **string receive a *string, nil meaning SQL NULL.
type fakeRow struct {
	values []*string
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, target := range dest {
		*(target.(**string)) = r.values[i]
	}
	return nil
}

func strPtr(value string) *string { return &value }

func TestRepository_CreateOAuthState_sendsEmptyOptionalFieldsAsNull(t *testing.T) {
	db := &fakeDB{}
	repo, err := New(db)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	expires := time.Date(2026, time.November, 1, 12, 10, 0, 0, time.UTC)
	if err := repo.CreateOAuthState(context.Background(), NewOAuthState{State: "abc", ExpiresAt: expires}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}
	if len(db.lastArgs) != 4 || db.lastArgs[0] != "abc" || db.lastArgs[1] != "" || db.lastArgs[2] != "" {
		t.Fatalf("unexpected query arguments: %v", db.lastArgs)
	}
}

func TestRepository_CreateOAuthState_wrapsDatabaseError(t *testing.T) {
	boom := errors.New("boom")
	repo, _ := New(&fakeDB{execErr: boom})

	err := repo.CreateOAuthState(context.Background(), NewOAuthState{State: "abc"})
	if !errors.Is(err, boom) {
		t.Fatalf("CreateOAuthState() = %v, want it to wrap the database error", err)
	}
}

func TestRepository_ConsumeOAuthState(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		row     fakeRow
		want    OAuthState
		wantErr error
	}{
		{
			name: "valid state with verifier and redirect",
			row:  fakeRow{values: []*string{strPtr("verifier"), strPtr("/dashboard")}},
			want: OAuthState{CodeVerifier: "verifier", RedirectTo: "/dashboard"},
		},
		{
			name: "null columns become empty strings",
			row:  fakeRow{values: []*string{nil, nil}},
			want: OAuthState{},
		},
		{
			name:    "unknown, expired or used state",
			row:     fakeRow{err: pgx.ErrNoRows},
			wantErr: ErrOAuthStateInvalid,
		},
		{
			name:    "database failure is not reported as an invalid state",
			row:     fakeRow{err: boom},
			wantErr: boom,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, _ := New(&fakeDB{row: test.row})

			got, err := repo.ConsumeOAuthState(context.Background(), "abc", time.Now())
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ConsumeOAuthState() error = %v, want %v", err, test.wantErr)
				}
				if test.wantErr == boom && errors.Is(err, ErrOAuthStateInvalid) {
					t.Fatal("a database failure must not look like an invalid state")
				}
				return
			}
			if err != nil {
				t.Fatalf("ConsumeOAuthState returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("ConsumeOAuthState() = %+v, want %+v", got, test.want)
			}
		})
	}
}
