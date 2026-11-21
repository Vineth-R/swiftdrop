//go:build integration && postgres

package courier

import (
	"errors"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestProofPhotoDatabaseAndLocalStorage(t *testing.T) {
	database := parceltest.New(t)
	storage, err := objectstore.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	parcelID := insertStatusParcel(t, database.Pool, "DELIVERED", statusCourierID, false)
	photo := proofIntegrationImage(t, "image/jpeg")
	proofService := NewProofService(database.Pool, storage, 0)
	result, err := proofService.CreateProof(t.Context(), ProofInput{
		ParcelID: parcelID, CourierID: statusCourierID,
		RecipientName: "Recipient", Photo: &ProofPhoto{Data: photo, MIMEType: "image/jpeg"},
	})
	if err != nil {
		t.Fatal(err)
	}

	photoService := NewProofPhotoService(database.Pool, storage, time.Minute)
	customer := PhotoViewer{UserID: statusCourierID}
	download, err := photoService.Photo(t.Context(), parcelID, customer)
	if err != nil {
		t.Fatal(err)
	}
	if !download.Stream || download.URL != "/api/v1/parcels/"+parcelID+"/proof/photo/content" {
		t.Fatalf("download = %+v", download)
	}
	body, contentType, size, err := photoService.Open(t.Context(), parcelID, customer)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if contentType != "image/jpeg" || size != int64(len(photo)) {
		t.Fatalf("contentType = %s, size = %d, want %d", contentType, size, len(photo))
	}

	if _, err := photoService.Photo(t.Context(), parcelID, PhotoViewer{UserID: statusParcelID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign customer error = %v", err)
	}
	if _, err := photoService.Photo(t.Context(), parcelID, PhotoViewer{UserID: statusCourierID, Courier: true}); err != nil {
		t.Fatalf("assigned courier error = %v", err)
	}
	if _, err := photoService.Photo(t.Context(), parcelID, PhotoViewer{UserID: statusParcelID, Courier: true}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign courier error = %v", err)
	}
	if _, err := photoService.Photo(t.Context(), parcelID, PhotoViewer{UserID: statusParcelID, Staff: true}); err != nil {
		t.Fatalf("staff error = %v", err)
	}

	noPhotoID := insertStatusParcel(t, database.Pool, "DELIVERED", statusCourierID, false)
	if _, err := photoService.Photo(t.Context(), noPhotoID, customer); !errors.Is(err, ErrProofPhotoNotFound) {
		t.Fatalf("missing photo error = %v, proof = %s", err, result.ID)
	}
}
