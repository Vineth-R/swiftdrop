package courier

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
)

type proofTestDB struct {
	tx          *proofTestTx
	beginErr    error
	probeErr    error
	probeExists bool
}

func (d *proofTestDB) Begin(context.Context) (pgx.Tx, error) { return d.tx, d.beginErr }
func (d *proofTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return proofTestRow{kind: "probe", exists: d.probeExists, err: d.probeErr}
}

type proofTestTx struct {
	pgx.Tx
	owner              *string
	assignment         bool
	lockErr, insertErr error
	commitErr          error
	createdAt          time.Time
	commits, rollbacks int
}

func (t *proofTestTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "FROM parcel.parcels") {
		return proofTestRow{kind: "lock", owner: t.owner, exists: t.assignment, err: t.lockErr}
	}
	return proofTestRow{kind: "insert", createdAt: t.createdAt, err: t.insertErr}
}
func (t *proofTestTx) Commit(context.Context) error   { t.commits++; return t.commitErr }
func (t *proofTestTx) Rollback(context.Context) error { t.rollbacks++; return pgx.ErrTxClosed }

type proofTestRow struct {
	kind      string
	owner     *string
	exists    bool
	createdAt time.Time
	err       error
}

func (r proofTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch r.kind {
	case "lock":
		*dest[0].(**string) = r.owner
		*dest[1].(*bool) = r.exists
	case "insert":
		*dest[0].(*time.Time) = r.createdAt
	case "probe":
		*dest[0].(*bool) = r.exists
	}
	return nil
}

type proofTestStorage struct {
	key                   string
	saveErr, deleteErr    error
	saves, deletes        int
	savedData             []byte
	savedMIME, deletedKey string
}

func (s *proofTestStorage) Save(_ context.Context, data []byte, mimeType string) (string, error) {
	s.saves++
	s.savedData, s.savedMIME = append([]byte(nil), data...), mimeType
	return s.key, s.saveErr
}
func (s *proofTestStorage) Delete(_ context.Context, key string) error {
	s.deletes++
	s.deletedKey = key
	return s.deleteErr
}

func validProofPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func proofServiceFixture(t *testing.T) (*ProofService, *proofTestTx, *proofTestStorage) {
	t.Helper()
	owner := courierTestOwner
	tx := &proofTestTx{owner: &owner, assignment: true, createdAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.FixedZone("test", 3600))}
	storage := &proofTestStorage{key: strings.Repeat("a", 32) + ".png"}
	return NewProofService(&proofTestDB{tx: tx}, storage), tx, storage
}

const courierTestOwner = "00000000-0000-4000-8000-000000000071"
const proofTestParcel = "00000000-0000-4000-8000-000000000072"

func TestCreateProofSuccess(t *testing.T) {
	service, tx, storage := proofServiceFixture(t)
	photo := validProofPNG(t)
	result, err := service.CreateProof(t.Context(), ProofInput{ParcelID: proofTestParcel, CourierID: courierTestOwner,
		RecipientName: " Amali ", Note: " At door ", Photo: &ProofPhoto{Data: photo, MIMEType: "image/png"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == "" || result.ParcelID != proofTestParcel || result.RecipientName == nil || *result.RecipientName != "Amali" || result.Note == nil || *result.Note != "At door" || !result.HasPhoto || result.CreatedAt.Location() != time.UTC {
		t.Fatalf("result = %+v", result)
	}
	if tx.commits != 1 || storage.saves != 1 || storage.deletes != 0 || storage.savedMIME != "image/png" || !bytes.Equal(storage.savedData, photo) {
		t.Fatalf("commit/storage = %d/%d/%d %s", tx.commits, storage.saves, storage.deletes, storage.savedMIME)
	}
}

func TestCreateProofValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input ProofInput
		want  error
	}{
		{name: "empty", input: ProofInput{}, want: ErrProofEmpty},
		{name: "recipient too long", input: ProofInput{RecipientName: strings.Repeat("x", 151)}, want: ErrProofValidation},
		{name: "note too long", input: ProofInput{Note: strings.Repeat("x", 501)}, want: ErrProofValidation},
		{name: "empty photo", input: ProofInput{Photo: &ProofPhoto{MIMEType: "image/png"}}, want: ErrProofUnsupported},
		{name: "oversized photo", input: ProofInput{Photo: &ProofPhoto{Data: make([]byte, proofmedia.MaxPhotoBytes+1), MIMEType: "image/png"}}, want: proofmedia.ErrPhotoTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, tx, storage := proofServiceFixture(t)
			tc.input.ParcelID, tc.input.CourierID = proofTestParcel, courierTestOwner
			_, err := service.CreateProof(t.Context(), tc.input)
			if !errors.Is(err, tc.want) || tx.commits != 0 || storage.saves != 0 {
				t.Fatalf("error=%v commits=%d saves=%d", err, tx.commits, storage.saves)
			}
		})
	}
}

func TestCreateProofAuthorizationBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		owner      *string
		assignment bool
		lockErr    error
		want       error
	}{
		{name: "missing", lockErr: pgx.ErrNoRows, want: ErrNotFound},
		{name: "unassigned", want: ErrForbidden},
		{name: "foreign", owner: stringPointerTest("00000000-0000-4000-8000-000000000099"), assignment: true, want: ErrForbidden},
		{name: "inconsistent", owner: stringPointerTest(courierTestOwner), want: ErrInconsistentAssignment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &proofTestTx{owner: tc.owner, assignment: tc.assignment, lockErr: tc.lockErr}
			storage := &proofTestStorage{key: strings.Repeat("a", 32) + ".png"}
			service := NewProofService(&proofTestDB{tx: tx}, storage)
			_, err := service.CreateProof(t.Context(), ProofInput{ParcelID: proofTestParcel, CourierID: courierTestOwner, Note: "note"})
			if !errors.Is(err, tc.want) || storage.saves != 0 || tx.commits != 0 {
				t.Fatalf("error=%v saves=%d commits=%d", err, storage.saves, tx.commits)
			}
		})
	}
}

func stringPointerTest(value string) *string { return &value }

func TestCreateProofCleansFileOnDefiniteFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*proofTestTx, *proofTestStorage)
	}{
		{name: "storage", set: func(_ *proofTestTx, storage *proofTestStorage) { storage.saveErr = errors.New("disk full") }},
		{name: "insert", set: func(tx *proofTestTx, _ *proofTestStorage) { tx.insertErr = errors.New("insert failed") }},
		{name: "commit rolled back", set: func(tx *proofTestTx, _ *proofTestStorage) { tx.commitErr = errors.New("commit failed") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := courierTestOwner
			tx := &proofTestTx{owner: &owner, assignment: true, createdAt: time.Now()}
			storage := &proofTestStorage{key: strings.Repeat("a", 32) + ".png"}
			db := &proofTestDB{tx: tx, probeExists: false}
			tc.set(tx, storage)
			service := NewProofService(db, storage)
			_, err := service.CreateProof(t.Context(), ProofInput{ParcelID: proofTestParcel, CourierID: courierTestOwner,
				Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"}})
			if err == nil {
				t.Fatal("failure succeeded")
			}
			wantDeletes := 1
			if tc.name == "storage" {
				wantDeletes = 0
			}
			if storage.deletes != wantDeletes {
				t.Fatalf("deletes=%d want=%d", storage.deletes, wantDeletes)
			}
		})
	}
}

func TestCreateProofResolvesCommittedResponseFailure(t *testing.T) {
	owner := courierTestOwner
	tx := &proofTestTx{owner: &owner, assignment: true, createdAt: time.Now(), commitErr: errors.New("connection lost")}
	storage := &proofTestStorage{key: strings.Repeat("a", 32) + ".png"}
	service := NewProofService(&proofTestDB{tx: tx, probeExists: true}, storage)
	result, err := service.CreateProof(t.Context(), ProofInput{ParcelID: proofTestParcel, CourierID: courierTestOwner,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"}})
	if err != nil || result.ID == "" || storage.deletes != 0 {
		t.Fatalf("result=%+v error=%v deletes=%d", result, err, storage.deletes)
	}
}
