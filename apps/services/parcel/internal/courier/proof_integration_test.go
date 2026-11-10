//go:build integration && postgres

package courier

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"

	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func proofIntegrationStorage(t *testing.T) (*proofmedia.LocalStorage, string) {
	t.Helper()
	directory := t.TempDir()
	storage, err := proofmedia.NewLocalStorage(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Error(err)
		}
	})
	return storage, directory
}

func proofIntegrationImage(t *testing.T, mimeType string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var data bytes.Buffer
	var err error
	if mimeType == "image/png" {
		err = png.Encode(&data, img)
	} else {
		err = jpeg.Encode(&data, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestCourierProofDatabase(t *testing.T) {
	database := parceltest.New(t)

	for _, mimeType := range []string{"image/jpeg", "image/png"} {
		t.Run(mimeType, func(t *testing.T) {
			storage, directory := proofIntegrationStorage(t)
			service := NewProofService(database.Pool, storage, 0)
			id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
			result, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID,
				RecipientName: " Recipient ", Note: " At door ",
				Photo: &ProofPhoto{Data: proofIntegrationImage(t, mimeType), MIMEType: mimeType}})
			if err != nil {
				t.Fatal(err)
			}
			var parcelID, recipient, note, path, storedMIME, capturedBy string
			var size int
			if err := database.Pool.QueryRow(t.Context(), `SELECT parcel_id,recipient_name,note,photo_path,
				photo_mime_type,photo_size_bytes,captured_by FROM parcel.delivery_proofs WHERE id=$1`, result.ID).
				Scan(&parcelID, &recipient, &note, &path, &storedMIME, &size, &capturedBy); err != nil {
				t.Fatal(err)
			}
			if parcelID != id || recipient != "Recipient" || note != "At door" || storedMIME != mimeType || size <= 0 || capturedBy != statusCourierID || strings.Contains(path, "/") || strings.Contains(path, `\`) {
				t.Fatalf("persisted proof = %s/%s/%s/%s/%s/%d/%s", parcelID, recipient, note, path, storedMIME, size, capturedBy)
			}
			if _, err := os.Stat(directory + string(os.PathSeparator) + path); err != nil {
				t.Fatalf("private proof file missing: %v", err)
			}
		})
	}

	for _, tc := range []struct {
		name, recipient, note string
	}{
		{name: "recipient only", recipient: "Recipient"},
		{name: "note only", note: "Left with security"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage, _ := proofIntegrationStorage(t)
			service := NewProofService(database.Pool, storage, 0)
			id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
			result, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID,
				RecipientName: tc.recipient, Note: tc.note})
			if err != nil || result.HasPhoto {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			var path *string
			if err := database.Pool.QueryRow(t.Context(), `SELECT photo_path FROM parcel.delivery_proofs WHERE id=$1`, result.ID).Scan(&path); err != nil || path != nil {
				t.Fatalf("photo path=%v error=%v", path, err)
			}
		})
	}

	t.Run("authorization and missing parcel write no files", func(t *testing.T) {
		storage, directory := proofIntegrationStorage(t)
		service := NewProofService(database.Pool, storage, 0)
		id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		for _, input := range []ProofInput{
			{ParcelID: id, CourierID: statusParcelID, Note: "foreign"},
			{ParcelID: parceltest.UUID(t), CourierID: statusCourierID, Note: "missing"},
		} {
			if _, err := service.CreateProof(t.Context(), input); !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrNotFound) {
				t.Fatalf("authorization error=%v", err)
			}
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("storage entries=%d error=%v", len(entries), err)
		}
	})

	t.Run("concurrent repeated uploads remain distinct", func(t *testing.T) {
		storage, directory := proofIntegrationStorage(t)
		service := NewProofService(database.Pool, storage, 0)
		id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		start := make(chan struct{})
		errorsCh := make(chan error, 2)
		photo := proofIntegrationImage(t, "image/png")
		var wait sync.WaitGroup
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID,
					Photo: &ProofPhoto{Data: photo, MIMEType: "image/png"}})
				errorsCh <- err
			}()
		}
		close(start)
		wait.Wait()
		close(errorsCh)
		for err := range errorsCh {
			if err != nil {
				t.Fatal(err)
			}
		}
		var count int
		if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1`, id).Scan(&count); err != nil || count != 2 {
			t.Fatalf("proof count=%d error=%v", count, err)
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 2 || entries[0].Name() == entries[1].Name() {
			t.Fatalf("storage entries=%v error=%v", entries, err)
		}
	})

	t.Run("database rejection removes stored file", func(t *testing.T) {
		storage, directory := proofIntegrationStorage(t)
		service := NewProofService(database.Pool, storage, 0)
		id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		// Fixture-only DDL uses a generated UUID, never request data.
		constraint := "ALTER TABLE parcel.delivery_proofs ADD CONSTRAINT m7_reject_proof CHECK (parcel_id <> '" + id + "'::uuid)"
		if _, err := database.Pool.Exec(t.Context(), constraint); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = database.Pool.Exec(context.Background(), "ALTER TABLE parcel.delivery_proofs DROP CONSTRAINT IF EXISTS m7_reject_proof")
		})
		_, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID,
			Photo: &ProofPhoto{Data: proofIntegrationImage(t, "image/png"), MIMEType: "image/png"}})
		if err == nil {
			t.Fatal("injected database failure succeeded")
		}
		entries, readErr := os.ReadDir(directory)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("orphan files=%v error=%v", entries, readErr)
		}
	})
}

func TestCourierProofParcelLimitDatabase(t *testing.T) {
	database := parceltest.New(t)

	t.Run("sequential limit", func(t *testing.T) {
		storage, _ := proofIntegrationStorage(t)
		service := NewProofService(database.Pool, storage, 2)
		id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		for attempt := 1; attempt <= 2; attempt++ {
			if _, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID, Note: "At door"}); err != nil {
				t.Fatalf("attempt %d: %v", attempt, err)
			}
		}
		if _, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID, Note: "At door"}); !errors.Is(err, ErrProofLimitReached) {
			t.Fatalf("limit error = %v", err)
		}
		var count int
		if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1`, id).Scan(&count); err != nil || count != 2 {
			t.Fatalf("proof count=%d error=%v", count, err)
		}
	})

	t.Run("concurrent limit", func(t *testing.T) {
		storage, _ := proofIntegrationStorage(t)
		service := NewProofService(database.Pool, storage, 1)
		id := insertStatusParcel(t, database.Pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wait sync.WaitGroup
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, err := service.CreateProof(t.Context(), ProofInput{ParcelID: id, CourierID: statusCourierID, Note: "At door"})
				results <- err
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		var succeeded, limited int
		for err := range results {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrProofLimitReached):
				limited++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if succeeded != 1 || limited != 1 {
			t.Fatalf("succeeded=%d limited=%d", succeeded, limited)
		}
		var count int
		if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1`, id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("proof count=%d error=%v", count, err)
		}
	})
}
