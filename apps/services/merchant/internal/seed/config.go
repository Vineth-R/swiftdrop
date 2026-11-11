// Package seed implements the local deterministic merchant demo seed command.
// It runs only when an operator invokes it; it is not part of service startup.
package seed

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/businessreg"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	defaultIdentityBaseURL = "http://localhost:8081"
	defaultIdentityTimeout = 3 * time.Second

	minFullNameLength = 2
	maxFullNameLength = 150
	minPasswordLength = 8
	maxPasswordLength = 72
)

// Config is the complete validated merchant demo-seed configuration.
type Config struct {
	AppEnv          string
	DatabaseURL     string
	IdentityBaseURL string
	IdentityTimeout time.Duration
	InternalToken   string

	MerchantEmail    string
	MerchantPassword string
	MerchantFullName string
	MerchantPhone    string

	BusinessName      string
	BusinessRegNo     string
	ContactEmail      string
	ContactPhone      string
	PickupAddress     string
	PickupZoneCode    string
	AllowRemoteTarget bool
}

// LoadConfig reads and validates the merchant demo seed from the environment.
// It never returns a configuration that targets production or, by default, a
// remote database.
func LoadConfig(loader sharedconfig.Loader) (Config, error) {
	appEnv, err := loader.EnumFold("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return Config{}, err
	}
	if appEnv == "production" {
		return Config{}, errors.New("refusing to run the merchant demo seed when APP_ENV=production")
	}

	databaseURL, err := loader.Required("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	allowRemote := strings.EqualFold(loader.String("ALLOW_REMOTE_SEED", ""), "1") ||
		strings.EqualFold(loader.String("ALLOW_REMOTE_SEED", ""), "true")

	identityBaseURL := strings.TrimRight(loader.String("IDENTITY_BASE_URL", defaultIdentityBaseURL), "/")
	if !validHTTPOrigin(identityBaseURL) {
		return Config{}, errors.New("IDENTITY_BASE_URL must be an absolute http or https URL")
	}

	identityTimeout, err := loader.Duration("IDENTITY_TIMEOUT", defaultIdentityTimeout)
	if err != nil {
		return Config{}, err
	}
	if identityTimeout < time.Millisecond {
		return Config{}, errors.New("IDENTITY_TIMEOUT must be at least 1ms")
	}

	internalToken, problem := sharedconfig.LoadInternalToken(loader, appEnv)
	if problem != "" {
		return Config{}, fmt.Errorf("INTERNAL_TOKEN is not usable: %s", problem)
	}

	merchantEmail, err := requiredEmail(loader, "SEED_MERCHANT_EMAIL")
	if err != nil {
		return Config{}, err
	}
	merchantPassword, err := loader.Required("SEED_MERCHANT_PASSWORD")
	if err != nil {
		return Config{}, errors.New("SEED_MERCHANT_PASSWORD is required; there is no default and the value is never echoed")
	}
	merchantFullName, err := requiredText(loader, "SEED_MERCHANT_FULL_NAME", minFullNameLength, maxFullNameLength, "must be 2 to 150 characters")
	if err != nil {
		return Config{}, err
	}
	merchantPhone, err := requiredPhone(loader, "SEED_MERCHANT_PHONE")
	if err != nil {
		return Config{}, err
	}

	businessName, err := requiredText(loader, "SEED_MERCHANT_BUSINESS_NAME", 2, 150, "must be 2 to 150 characters")
	if err != nil {
		return Config{}, err
	}
	businessRegNo, err := loader.Required("SEED_MERCHANT_BUSINESS_REG_NO")
	if err != nil {
		return Config{}, err
	}
	normalizedRegNo, regErr := businessreg.NormalizeValid(businessRegNo)
	if regErr != nil {
		return Config{}, errors.New("SEED_MERCHANT_BUSINESS_REG_NO must be 3 to 50 characters with at least one digit")
	}
	contactEmail, err := requiredEmail(loader, "SEED_MERCHANT_CONTACT_EMAIL")
	if err != nil {
		return Config{}, err
	}
	contactPhone, err := requiredPhone(loader, "SEED_MERCHANT_CONTACT_PHONE")
	if err != nil {
		return Config{}, err
	}
	pickupAddress, err := requiredText(loader, "SEED_MERCHANT_PICKUP_ADDRESS", 5, 255, "must be 5 to 255 characters")
	if err != nil {
		return Config{}, err
	}
	pickupZoneCode, err := loader.Required("SEED_MERCHANT_PICKUP_ZONE_CODE")
	if err != nil {
		return Config{}, err
	}

	config := Config{
		AppEnv:          appEnv,
		DatabaseURL:     databaseURL,
		IdentityBaseURL: identityBaseURL,
		IdentityTimeout: identityTimeout,
		InternalToken:   internalToken,

		MerchantEmail:    merchantEmail,
		MerchantPassword: merchantPassword,
		MerchantFullName: merchantFullName,
		MerchantPhone:    merchantPhone,

		BusinessName:      businessName,
		BusinessRegNo:     normalizedRegNo,
		ContactEmail:      contactEmail,
		ContactPhone:      contactPhone,
		PickupAddress:     pickupAddress,
		PickupZoneCode:    strings.ToUpper(strings.TrimSpace(pickupZoneCode)),
		AllowRemoteTarget: allowRemote,
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks the complete seed configuration before any network or
// database work. Errors never contain a password, token or database URL.
func (c Config) Validate() error {
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") {
		return errors.New("refusing to run the merchant demo seed when APP_ENV=production")
	}

	host, err := databaseHost(c.DatabaseURL)
	if err != nil {
		return err
	}
	if !isLocalHost(host) && !c.AllowRemoteTarget {
		return fmt.Errorf("refusing to seed a non-local database host %q; set ALLOW_REMOTE_SEED=1 only for a disposable database", host)
	}

	if !validation.Email(strings.TrimSpace(c.MerchantEmail)) {
		return errors.New("SEED_MERCHANT_EMAIL must be a valid email address")
	}
	if !validPassword(c.MerchantPassword) {
		return errors.New("SEED_MERCHANT_PASSWORD must be 8 to 72 characters with upper case, lower case and a digit")
	}
	if !validName(c.MerchantFullName) {
		return errors.New("SEED_MERCHANT_FULL_NAME must be 2 to 150 characters")
	}
	if !validPhone(c.MerchantPhone) {
		return errors.New("SEED_MERCHANT_PHONE must be in E.164 format, for example +94771234567")
	}
	if !validName(c.BusinessName) {
		return errors.New("SEED_MERCHANT_BUSINESS_NAME must be 2 to 150 characters")
	}
	if _, err := businessreg.NormalizeValid(c.BusinessRegNo); err != nil {
		return errors.New("SEED_MERCHANT_BUSINESS_REG_NO must be 3 to 50 characters with at least one digit")
	}
	if !validation.Email(strings.TrimSpace(c.ContactEmail)) {
		return errors.New("SEED_MERCHANT_CONTACT_EMAIL must be a valid email address")
	}
	if !validPhone(c.ContactPhone) {
		return errors.New("SEED_MERCHANT_CONTACT_PHONE must be in E.164 format, for example +94771234567")
	}
	if validation.RuneLength(c.PickupAddress) < 5 || validation.RuneLength(c.PickupAddress) > 255 {
		return errors.New("SEED_MERCHANT_PICKUP_ADDRESS must be 5 to 255 characters")
	}
	if strings.TrimSpace(c.PickupZoneCode) == "" {
		return errors.New("SEED_MERCHANT_PICKUP_ZONE_CODE is required")
	}
	return nil
}

func databaseHost(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return "", errors.New("DATABASE_URL must be a valid PostgreSQL URL")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", errors.New("DATABASE_URL must include a host")
	}
	return host, nil
}

func isLocalHost(host string) bool {
	normalized := strings.Trim(strings.ToLower(host), "[]")
	if normalized == "localhost" {
		return true
	}
	if ip := net.ParseIP(normalized); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func validHTTPOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func requiredEmail(loader sharedconfig.Loader, key string) (string, error) {
	value, err := loader.Required(key)
	if err != nil {
		return "", err
	}
	email := strings.ToLower(strings.TrimSpace(value))
	if !validation.Email(email) {
		return "", fmt.Errorf("%s must be a valid email address", key)
	}
	return email, nil
}

func requiredText(loader sharedconfig.Loader, key string, min, max int, issue string) (string, error) {
	value, err := loader.Required(key)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(value)
	if validation.RuneLength(trimmed) < min || validation.RuneLength(trimmed) > max {
		return "", fmt.Errorf("%s %s", key, issue)
	}
	return trimmed, nil
}

func requiredPhone(loader sharedconfig.Loader, key string) (string, error) {
	value, err := loader.Required(key)
	if err != nil {
		return "", err
	}
	phone := strings.TrimSpace(value)
	if !validPhone(phone) {
		return "", fmt.Errorf("%s must be in E.164 format, for example +94771234567", key)
	}
	return phone, nil
}

func validName(value string) bool {
	length := validation.RuneLength(strings.TrimSpace(value))
	return length >= minFullNameLength && length <= maxFullNameLength
}

func validPhone(value string) bool {
	if len(value) < 3 || len(value) > 16 || value[0] != '+' || value[1] == '0' {
		return false
	}
	for _, char := range value[1:] {
		if !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func validPassword(value string) bool {
	if utf8.RuneCountInString(value) < minPasswordLength || len(value) > maxPasswordLength {
		return false
	}
	var upper, lower, digit bool
	for _, char := range value {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
			digit = true
		}
	}
	return upper && lower && digit
}
