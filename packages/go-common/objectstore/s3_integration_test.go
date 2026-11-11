//go:build integration && storage

package objectstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	endpoint := os.Getenv("STORAGE_TEST_ENDPOINT")
	access := os.Getenv("STORAGE_TEST_ACCESS_KEY")
	secret := os.Getenv("STORAGE_TEST_SECRET_KEY")
	if endpoint == "" || access == "" || secret == "" {
		t.Fatal("STORAGE_TEST_ENDPOINT, STORAGE_TEST_ACCESS_KEY and STORAGE_TEST_SECRET_KEY are required")
	}
	bucket := os.Getenv("STORAGE_TEST_BUCKET")
	if bucket == "" {
		bucket = "delivery-proofs-test"
	}
	region := os.Getenv("STORAGE_TEST_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return Config{Provider: ProviderS3, Endpoint: endpoint, Region: region, Bucket: bucket,
		AccessKeyID: access, SecretAccessKey: secret, UsePathStyle: true, PresignTTL: 5 * time.Minute}
}

func ensureBucket(t *testing.T, config Config) {
	t.Helper()
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(config.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(config.Endpoint)
		options.UsePathStyle = true
	})
	if _, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(config.Bucket)}); err != nil {
		// S3-compatible endpoints return BucketAlreadyOwnedByYou/BucketAlreadyExists on repeat runs.
		if !strings.Contains(err.Error(), "BucketAlready") {
			t.Fatal(err)
		}
	}
}

func TestS3PrivateLifecycle(t *testing.T) {
	config := testConfig(t)
	ensureBucket(t, config)
	store, err := NewS3(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	key := testParcelID + "/" + testProofID + ".jpg"
	data := []byte("private proof bytes for S3-compatible storage")
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })

	if err := store.Put(t.Context(), key, data, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	exists, err := store.Exists(t.Context(), key)
	if err != nil || !exists {
		t.Fatalf("exists = %v, error = %v", exists, err)
	}
	info, err := store.Stat(t.Context(), key)
	if err != nil || info.ContentType != "image/jpeg" || info.Size != int64(len(data)) {
		t.Fatalf("stat = %+v, error = %v", info, err)
	}

	presigned, _, err := store.PresignGet(t.Context(), key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(presigned)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || err != nil || string(body) != string(data) {
		t.Fatalf("status = %d, error = %v, body = %q", response.StatusCode, err, body)
	}

	object, err := store.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()

	missing := testParcelID + "/" + "00000000-0000-4000-8000-000000000099.jpg"
	if exists, err := store.Exists(t.Context(), missing); err != nil || exists {
		t.Fatalf("missing exists = %v, error = %v", exists, err)
	}
	if _, err := store.Open(t.Context(), missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open(missing) = %v", err)
	}
}

func TestS3PresignedURLExpires(t *testing.T) {
	config := testConfig(t)
	ensureBucket(t, config)
	store, err := NewS3(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	key := testParcelID + "/" + testProofID + ".png"
	if err := store.Put(t.Context(), key, []byte("expiring"), "image/png"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })

	presigned, _, err := store.PresignGet(t.Context(), key, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	response, err := http.Get(presigned)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expired status = %d", response.StatusCode)
	}
}
