package objectstore

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const (
	ProviderLocal = "local"
	ProviderS3    = "s3"
	ProviderNeon  = "neon"
	ProviderMinIO = "minio"

	DefaultPresignTTL = 5 * time.Minute
	MinPresignTTL     = 30 * time.Second
	MaxPresignTTL     = time.Hour
)

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Config is provider configuration only. It never contains a bucket policy or
// public URL, and it is never logged.
type Config struct {
	Provider        string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	LocalDirectory  string
	PresignTTL      time.Duration
}

// Enabled reports whether a provider is configured.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Provider) != ""
}

// LoadConfig parses STORAGE_* and proof-specific TTL settings.
func LoadConfig(loader sharedconfig.Loader) (Config, error) {
	provider := strings.ToLower(loader.String("STORAGE_PROVIDER", ""))
	localDirectory := loader.String("STORAGE_LOCAL_DIR", loader.String("PROOF_STORAGE_DIR", ""))
	if provider == "" {
		if localDirectory != "" {
			provider = ProviderLocal
		} else {
			return Config{}, nil
		}
	}

	presignTTLSeconds, err := loader.Int("PROOF_PRESIGNED_URL_TTL_SECONDS", int(DefaultPresignTTL/time.Second))
	if err != nil {
		return Config{}, err
	}
	if presignTTLSeconds < int(MinPresignTTL/time.Second) || presignTTLSeconds > int(MaxPresignTTL/time.Second) {
		return Config{}, fmt.Errorf("PROOF_PRESIGNED_URL_TTL_SECONDS must be between %d and %d", int(MinPresignTTL/time.Second), int(MaxPresignTTL/time.Second))
	}

	config := Config{
		Provider:        provider,
		Endpoint:        strings.TrimRight(loader.String("STORAGE_ENDPOINT", ""), "/"),
		Region:          loader.String("STORAGE_REGION", "us-east-2"),
		Bucket:          loader.String("STORAGE_BUCKET", "delivery-proofs"),
		AccessKeyID:     loader.String("STORAGE_ACCESS_KEY_ID", ""),
		SecretAccessKey: loader.String("STORAGE_SECRET_ACCESS_KEY", ""),
		LocalDirectory:  localDirectory,
		PresignTTL:      time.Duration(presignTTLSeconds) * time.Second,
	}

	switch provider {
	case ProviderLocal:
		if localDirectory == "" {
			return Config{}, fmt.Errorf("%w: STORAGE_LOCAL_DIR or PROOF_STORAGE_DIR is required for the local provider", ErrInvalidConfig)
		}
		return config, nil
	case ProviderNeon, ProviderMinIO, ProviderS3:
	default:
		return Config{}, fmt.Errorf("%w: STORAGE_PROVIDER must be local, s3, neon or minio", ErrInvalidConfig)
	}

	if config.Endpoint == "" && provider != ProviderS3 {
		return Config{}, fmt.Errorf("%w: STORAGE_ENDPOINT is required for the %s provider", ErrInvalidConfig, provider)
	}
	if config.Endpoint != "" {
		parsed, parseErr := url.Parse(config.Endpoint)
		if parseErr != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, fmt.Errorf("%w: STORAGE_ENDPOINT must be an HTTP(S) origin", ErrInvalidConfig)
		}
	}
	if strings.TrimSpace(config.Region) == "" {
		return Config{}, fmt.Errorf("%w: STORAGE_REGION is required", ErrInvalidConfig)
	}
	if !bucketPattern.MatchString(config.Bucket) || strings.Contains(config.Bucket, "..") {
		return Config{}, fmt.Errorf("%w: STORAGE_BUCKET must be a valid private bucket name", ErrInvalidConfig)
	}
	if config.AccessKeyID == "" || config.SecretAccessKey == "" {
		return Config{}, fmt.Errorf("%w: STORAGE_ACCESS_KEY_ID and STORAGE_SECRET_ACCESS_KEY are required", ErrInvalidConfig)
	}

	usePathStyle := provider != ProviderS3
	if raw := loader.String("STORAGE_USE_PATH_STYLE", ""); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return Config{}, fmt.Errorf("STORAGE_USE_PATH_STYLE must be true or false")
		}
		usePathStyle = value
	}
	// Neon and MinIO use path-style addressing only. S3 accepts either.
	if (provider == ProviderNeon || provider == ProviderMinIO) && !usePathStyle {
		return Config{}, fmt.Errorf("%w: the %s provider requires STORAGE_USE_PATH_STYLE=true", ErrInvalidConfig, provider)
	}
	config.UsePathStyle = usePathStyle

	return config, nil
}
