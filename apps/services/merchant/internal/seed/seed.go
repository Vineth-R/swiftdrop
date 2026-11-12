package seed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/google/uuid"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/businessreg"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// seedAdminActorID is a deterministic synthetic administrator identity used
// only as the approved_by/audit actor for a seeded merchant. It is not a real
// identity user and is never granted a session.
const seedAdminActorID = "00000000-0000-4000-8000-000000000001"

// errIdentityEmailExists means Identity returned an existing account that this
// seed did not create. The seed refuses to attach a merchant to it.
var errIdentityEmailExists = errors.New("identity account already exists without a merchant")

// Repository is the merchant persistence surface the seed needs. It embeds the
// onboarding store so the real registration and approval invariants are reused.
type Repository interface {
	onboarding.Store
	MerchantByContactEmail(ctx context.Context, contactEmail string) (repository.Merchant, error)
	APIClientByLookup(ctx context.Context, lookupID string) (repository.APIClient, string, error)
	CreateAPIClient(ctx context.Context, params repository.CreateAPIClientParams) error
}

// ZoneSource resolves the pickup zone code from pricing.zones. It is read-only.
type ZoneSource interface {
	ZoneByCode(ctx context.Context, code string) (pricingclient.Zone, error)
}

// Result reports what the seed created or found.
type Result struct {
	Merchant      repository.Merchant
	AlreadySeeded bool

	// APIKeyCreated is true when the run inserted the configured demo key.
	// APIKeyPrefix is safe to log; the full key never leaves the caller.
	APIKeyCreated bool
	APIKeyPrefix  string
}

// Run creates a MERCHANT identity owner through Identity's internal API and a
// merchant row through the Merchant service's own registration and approval
// path. A completed run leaves the merchant APPROVED. Reruns are no-ops: an
// existing approved merchant and its identity user are never updated.
func Run(ctx context.Context, repo Repository, identity onboarding.Identity, zones ZoneSource, config Config) (Result, error) {
	if repo == nil {
		return Result{}, errors.New("merchant seed requires a repository")
	}
	if identity == nil {
		return Result{}, errors.New("merchant seed requires an identity client")
	}
	if zones == nil {
		return Result{}, errors.New("merchant seed requires a pricing zone source")
	}
	if err := config.Validate(); err != nil {
		return Result{}, err
	}

	zone, err := zones.ZoneByCode(ctx, config.PickupZoneCode)
	if err != nil {
		return Result{}, err
	}
	if !zone.IsActive {
		return Result{}, fmt.Errorf("pickup zone %s is not active", config.PickupZoneCode)
	}

	recorder := &recordingIdentity{Identity: identity}
	service, err := onboarding.New(
		guardedStore{Store: repo, identity: recorder},
		recorder,
		staticZoner{zone: zone},
		nil,
		nil,
	)
	if err != nil {
		return Result{}, fmt.Errorf("create onboarding service: %w", err)
	}

	existing, err := repo.MerchantByContactEmail(ctx, config.ContactEmail)
	switch {
	case err == nil:
		return completeExisting(ctx, repo, service, existing, config)
	case !errors.Is(err, repository.ErrNotFound):
		return Result{}, fmt.Errorf("look up seeded merchant: %w", err)
	}

	registered, err := service.Register(ctx, onboarding.RegisterInput{
		FullName: config.MerchantFullName,
		Email:    config.MerchantEmail,
		Phone:    config.MerchantPhone,
		Password: config.MerchantPassword,
		Business: onboarding.BusinessInput{
			Name:               config.BusinessName,
			RegistrationNumber: config.BusinessRegNo,
			ContactEmail:       config.ContactEmail,
			ContactPhone:       config.ContactPhone,
			// The demo seed has no separate business-address variable, so the
			// pickup address is the deterministic business address too.
			BusinessAddress: config.PickupAddress,
			PickupAddress:   config.PickupAddress,
			PickupZoneID:    zone.ID,
		},
	})
	var validationErr *onboarding.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return Result{}, fmt.Errorf("invalid merchant seed fields: %s", issueSummary(validationErr.Issues))
	case errors.Is(err, errIdentityEmailExists):
		return Result{}, fmt.Errorf("identity account %s already exists without a merchant; refusing to attach it. Use a different SEED_MERCHANT_EMAIL or resolve the existing account", config.MerchantEmail)
	case errors.Is(err, onboarding.ErrEmailInUse):
		return Result{}, fmt.Errorf("identity account %s is already registered without a matching seeded merchant; refusing to overwrite it. Use a different SEED_MERCHANT_EMAIL", config.MerchantEmail)
	case errors.Is(err, onboarding.ErrDuplicateApplication):
		return Result{}, fmt.Errorf("a merchant application already exists for %s; use the same SEED_MERCHANT_BUSINESS_REG_NO or inspect the existing merchant", config.MerchantEmail)
	case err != nil:
		return Result{}, err
	}

	if registered.Status == repository.MerchantStatusApproved {
		merchant, err := repo.MerchantByID(ctx, registered.MerchantID)
		if err != nil {
			return Result{}, fmt.Errorf("read seeded merchant: %w", err)
		}
		return finish(ctx, repo, merchant, true, config)
	}

	approval, err := service.Approve(ctx, registered.MerchantID, seedAdminActorID)
	if err != nil {
		return Result{}, fmt.Errorf("approve seeded merchant: %w", err)
	}
	return finish(ctx, repo, approval.Merchant, false, config)
}

// finish ensures the optional demo API key exists and reports its non-secret
// prefix. It never stores or logs the full key.
func finish(ctx context.Context, repo Repository, merchant repository.Merchant, already bool, config Config) (Result, error) {
	result := Result{Merchant: merchant, AlreadySeeded: already}
	if config.MerchantAPIKey == "" {
		return result, nil
	}

	lookupID, err := apikey.Parse(config.MerchantAPIKey)
	if err != nil {
		return Result{}, errors.New("SEED_MERCHANT_API_KEY is not a valid merchant API key")
	}
	expectedHash := apikey.Hash(config.MerchantAPIKey)

	existing, storedHash, err := repo.APIClientByLookup(ctx, lookupID)
	switch {
	case err == nil:
		if existing.MerchantID != merchant.ID || !apikey.EqualHash(storedHash, expectedHash) {
			return Result{}, errors.New("SEED_MERCHANT_API_KEY collides with an existing key owned by another merchant; use a new key")
		}
		result.APIKeyPrefix = existing.KeyPrefix
		return result, nil
	case !errors.Is(err, repository.ErrNotFound):
		return Result{}, fmt.Errorf("look up seeded API key: %w", err)
	}

	prefix := apikey.Prefix + lookupID[:8]
	if err := repo.CreateAPIClient(ctx, repository.CreateAPIClientParams{
		ID:         uuid.NewString(),
		MerchantID: merchant.ID,
		Name:       "Demo API key",
		KeyPrefix:  prefix,
		LookupID:   lookupID,
		KeyHash:    expectedHash,
	}); err != nil {
		return Result{}, fmt.Errorf("create seeded API key: %w", err)
	}
	result.APIKeyCreated = true
	result.APIKeyPrefix = prefix
	return result, nil
}

func issueSummary(issues []onboarding.ValidationIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Field+": "+issue.Issue)
	}
	if len(parts) == 0 {
		return "validation failed"
	}
	return strings.Join(parts, "; ")
}

func completeExisting(ctx context.Context, repo Repository, service *onboarding.Service, merchant repository.Merchant, config Config) (Result, error) {
	switch merchant.Status {
	case repository.MerchantStatusApproved:
		return finish(ctx, repo, merchant, true, config)
	case repository.MerchantStatusPending:
		if merchant.BusinessRegNo == nil || businessreg.Normalize(*merchant.BusinessRegNo) != config.BusinessRegNo {
			return Result{}, fmt.Errorf("contact email %s already belongs to a different merchant; use a different SEED_MERCHANT_CONTACT_EMAIL", config.ContactEmail)
		}
		approval, err := service.Approve(ctx, merchant.ID, seedAdminActorID)
		if err != nil {
			return Result{}, fmt.Errorf("approve pending seeded merchant: %w", err)
		}
		return finish(ctx, repo, approval.Merchant, false, config)
	default:
		return Result{}, fmt.Errorf("merchant %s is %s; refusing to modify an existing merchant", merchant.ID, merchant.Status)
	}
}

// recordingIdentity remembers whether Identity created the owner. onboarding
// resumes an existing account when the password matches, so this lets the seed
// refuse to attach a merchant to an account it did not create.
type recordingIdentity struct {
	onboarding.Identity
	attempted bool
	created   bool
}

func (r *recordingIdentity) CreateMerchantOwner(ctx context.Context, input identityclient.OwnerInput) (identityclient.OwnerResult, error) {
	r.attempted = true
	result, err := r.Identity.CreateMerchantOwner(ctx, input)
	r.created = result.Created
	return result, err
}

// guardedStore stops CreatePendingMerchant when Identity resumed an account
// this seed did not create, before any merchant row is written.
type guardedStore struct {
	onboarding.Store
	identity *recordingIdentity
}

func (g guardedStore) CreatePendingMerchant(ctx context.Context, attemptID string, params repository.CreateMerchantParams) (repository.Merchant, error) {
	if g.identity != nil && g.identity.attempted && !g.identity.created {
		return repository.Merchant{}, errIdentityEmailExists
	}
	return g.Store.CreatePendingMerchant(ctx, attemptID, params)
}

type staticZoner struct {
	zone pricingclient.Zone
}

func (z staticZoner) Zone(_ context.Context, zoneID string) (pricingclient.Zone, error) {
	if strings.TrimSpace(zoneID) != z.zone.ID {
		return pricingclient.Zone{}, errors.New("pickup zone does not match the seeded zone")
	}
	if !z.zone.IsActive {
		return pricingclient.Zone{}, errors.New("pickup zone is not active")
	}
	return z.zone, nil
}

// PostgresZoneSource reads pricing.zones by code. The seed never writes pricing
// data; this is the same read-only cross-schema dependency the identity seed
// uses for courier home zones.
type PostgresZoneSource struct {
	pool *postgres.Pool
}

func NewPostgresZoneSource(pool *postgres.Pool) *PostgresZoneSource {
	return &PostgresZoneSource{pool: pool}
}

func (s *PostgresZoneSource) ZoneByCode(ctx context.Context, code string) (pricingclient.Zone, error) {
	if s == nil || s.pool == nil {
		return pricingclient.Zone{}, errors.New("zone lookup requires a database")
	}
	zone := pricingclient.Zone{}
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, code, name, is_active
		FROM pricing.zones
		WHERE upper(code) = upper($1)`, strings.TrimSpace(code),
	).Scan(&zone.ID, &zone.Code, &zone.Name, &zone.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricingclient.Zone{}, fmt.Errorf("pickup zone %s is not configured", code)
	}
	if err != nil {
		return pricingclient.Zone{}, fmt.Errorf("read pickup zone %s: %w", code, err)
	}
	if !zone.IsActive {
		return pricingclient.Zone{}, fmt.Errorf("pickup zone %s is not active", code)
	}
	return zone, nil
}
