package courier

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
)

const photoParcelID = "00000000-0000-4000-8000-000000000081"
const photoCustomerID = "00000000-0000-4000-8000-000000000082"
const photoCourierID = "00000000-0000-4000-8000-000000000083"
const photoKey = photoParcelID + "/00000000-0000-4000-8000-000000000084.jpg"

type photoTestDB struct {
	customer   *string
	courier    *string
	assignment bool
	photoKey   *string
	mime       *string
	parcelErr  error
	photoErr   error
}

func (d *photoTestDB) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("not used") }
func (d *photoTestDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "FROM parcel.parcels") {
		return photoTestRow{kind: "parcel", customer: d.customer, courier: d.courier, assignment: d.assignment, err: d.parcelErr}
	}
	return photoTestRow{kind: "photo", key: d.photoKey, mime: d.mime, err: d.photoErr}
}

type photoTestRow struct {
	kind       string
	customer   *string
	courier    *string
	assignment bool
	key        *string
	mime       *string
	err        error
}

func (r photoTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.kind == "parcel" {
		*dest[0].(**string) = r.customer
		*dest[1].(**string) = r.courier
		*dest[2].(*bool) = r.assignment
		return nil
	}
	*dest[0].(*string) = *r.key
	*dest[1].(**string) = r.mime
	return nil
}

type photoTestStorage struct {
	presignErr error
	openErr    error
	key        string
	content    string
}

func (s *photoTestStorage) Put(context.Context, string, []byte, string) error { return nil }
func (s *photoTestStorage) Delete(context.Context, string) error              { return nil }
func (s *photoTestStorage) Exists(context.Context, string) (bool, error)      { return true, nil }
func (s *photoTestStorage) PresignGet(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	s.key = key
	if s.presignErr != nil {
		return "", time.Time{}, s.presignErr
	}
	return "https://storage.example/" + key + "?sig=short", time.Now().Add(ttl), nil
}
func (s *photoTestStorage) Open(context.Context, string) (*objectstore.Object, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return &objectstore.Object{Body: io.NopCloser(strings.NewReader(s.content)), Size: int64(len(s.content)), ContentType: "image/jpeg"}, nil
}
func (s *photoTestStorage) Stat(context.Context, string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, objectstore.ErrNotFound
}
func (s *photoTestStorage) List(context.Context, string) ([]string, error) { return nil, nil }
func (s *photoTestStorage) Close() error                                   { return nil }

func pointer(value string) *string { return &value }

func photoFixture(t *testing.T) (*ProofPhotoService, *photoTestDB, *photoTestStorage) {
	t.Helper()
	db := &photoTestDB{customer: pointer(photoCustomerID), courier: pointer(photoCourierID), assignment: true,
		photoKey: pointer(photoKey), mime: pointer("image/jpeg")}
	storage := &photoTestStorage{}
	return NewProofPhotoService(db, storage, 5*time.Minute), db, storage
}

func TestProofPhotoAuthorization(t *testing.T) {
	tests := []struct {
		name    string
		viewer  PhotoViewer
		mutate  func(*photoTestDB)
		wantErr error
	}{
		{name: "customer owner", viewer: PhotoViewer{UserID: photoCustomerID}},
		{name: "assigned courier", viewer: PhotoViewer{UserID: photoCourierID, Courier: true}},
		{name: "staff", viewer: PhotoViewer{UserID: "00000000-0000-4000-8000-000000000099", Staff: true}},
		{name: "foreign customer", viewer: PhotoViewer{UserID: "00000000-0000-4000-8000-000000000099"}, wantErr: ErrNotFound},
		{name: "foreign courier", viewer: PhotoViewer{UserID: "00000000-0000-4000-8000-000000000099", Courier: true}, wantErr: ErrForbidden},
		{name: "courier without current assignment", viewer: PhotoViewer{UserID: photoCourierID, Courier: true},
			mutate: func(db *photoTestDB) { db.assignment = false }, wantErr: ErrForbidden},
		{name: "missing parcel", viewer: PhotoViewer{UserID: photoCustomerID},
			mutate: func(db *photoTestDB) { db.parcelErr = pgx.ErrNoRows }, wantErr: ErrNotFound},
		{name: "missing photo", viewer: PhotoViewer{UserID: photoCustomerID},
			mutate: func(db *photoTestDB) { db.photoErr = pgx.ErrNoRows }, wantErr: ErrProofPhotoNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, db, _ := photoFixture(t)
			if tc.mutate != nil {
				tc.mutate(db)
			}
			download, err := service.Photo(t.Context(), photoParcelID, tc.viewer)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if download.URL == "" || download.MIMEType != "image/jpeg" || download.Stream {
				t.Fatalf("download = %+v", download)
			}
		})
	}
}

func TestProofPhotoLocalStreamFallback(t *testing.T) {
	service, _, storage := photoFixture(t)
	storage.presignErr = objectstore.ErrPresignUnsupported
	download, err := service.Photo(t.Context(), photoParcelID, PhotoViewer{UserID: photoCustomerID})
	if err != nil {
		t.Fatal(err)
	}
	if !download.Stream || download.URL != "/api/v1/parcels/"+photoParcelID+"/proof/photo/content" {
		t.Fatalf("download = %+v", download)
	}
}

func TestProofPhotoOpenMissingObject(t *testing.T) {
	service, _, storage := photoFixture(t)
	storage.openErr = objectstore.ErrNotFound
	if _, _, _, err := service.Open(t.Context(), photoParcelID, PhotoViewer{UserID: photoCustomerID}); !errors.Is(err, ErrProofPhotoNotFound) {
		t.Fatalf("Open() = %v", err)
	}
}
