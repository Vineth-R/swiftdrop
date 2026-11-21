package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3 is the S3-compatible adapter used by Neon Object Storage and MinIO.
// Provider details stay here; callers only see the Store interface.
type S3 struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

// NewS3 builds a path-style-aware S3 client for a private bucket.
func NewS3(ctx context.Context, config Config) (*S3, error) {
	if !config.Enabled() || config.Provider == ProviderLocal {
		return nil, ErrUnavailable
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(config.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			config.AccessKeyID, config.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("configure object storage client: %w", err)
	}
	// Presigned PUT URLs must not embed a checksum computed from an empty body.
	// Uploads here are server-side, but the setting keeps presigning safe if
	// write URLs are added later.
	awsConfig.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired

	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		if config.Endpoint != "" {
			options.BaseEndpoint = aws.String(config.Endpoint)
		}
		options.UsePathStyle = config.UsePathStyle
	})
	return &S3{client: client, presigner: s3.NewPresignClient(client), bucket: config.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	if !ValidKey(key) {
		return ErrUnsafeKey
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	if !ValidKey(key) {
		return ErrUnsafeKey
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.client == nil {
		return false, ErrUnavailable
	}
	if !ValidKey(key) {
		return false, ErrUnsafeKey
	}
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("head object: %w", err)
}

func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	if s == nil || s.presigner == nil {
		return "", time.Time{}, ErrUnavailable
	}
	if !ValidKey(key) {
		return "", time.Time{}, ErrUnsafeKey
	}
	request, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign object: %w", err)
	}
	return request.URL, time.Now().Add(ttl).UTC(), nil
}

func (s *S3) Open(ctx context.Context, key string) (*Object, error) {
	if s == nil || s.client == nil {
		return nil, ErrUnavailable
	}
	if !ValidKey(key) {
		return nil, ErrUnsafeKey
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get object: %w", err)
	}
	var size int64
	if output.ContentLength != nil {
		size = *output.ContentLength
	}
	var contentType string
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	return &Object{Body: output.Body, Size: size, ContentType: contentType}, nil
}

func (s *S3) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	if s == nil || s.client == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if !ValidKey(key) {
		return ObjectInfo{}, ErrUnsafeKey
	}
	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, fmt.Errorf("head object: %w", err)
	}
	var size int64
	if output.ContentLength != nil {
		size = *output.ContentLength
	}
	var contentType string
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	return ObjectInfo{Size: size, ContentType: contentType}, nil
}

func (s *S3) List(ctx context.Context, prefix string) ([]string, error) {
	if s == nil || s.client == nil {
		return nil, ErrUnavailable
	}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	keys := []string{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, object := range page.Contents {
			if object.Key != nil {
				keys = append(keys, *object.Key)
			}
		}
	}
	return keys, nil
}

func (s *S3) Close() error { return nil }

func isNotFound(err error) bool {
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket":
			return true
		}
	}
	return false
}

// New creates the configured provider. A disabled configuration returns
// ErrUnavailable so services can keep routes registered but fail closed.
func New(ctx context.Context, config Config) (Store, error) {
	switch config.Provider {
	case "":
		return nil, ErrUnavailable
	case ProviderLocal:
		return NewLocal(config.LocalDirectory)
	case ProviderS3, ProviderNeon, ProviderMinIO:
		return NewS3(ctx, config)
	default:
		return nil, fmt.Errorf("%w: unknown provider", ErrInvalidConfig)
	}
}
