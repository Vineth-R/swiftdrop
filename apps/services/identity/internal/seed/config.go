// Package seed implements the local deterministic demo-user seed command. It
// runs only when an operator invokes it; it is not part of service startup.
package seed

import (
	"errors"
	"fmt"
	"strings"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	courierOneHomeZone = "CMB"
	courierTwoHomeZone = "KDY"

	maxFullNameLength    = 150
	minFullNameLength    = 2
	maxVehicleTypeLength = 30
	maxVehicleNumberLen  = 20
)

// Account is one demo account loaded from the environment.
type Account struct {
	Role          auth.Role
	Email         string
	Password      string
	FullName      string
	HomeZoneCode  string
	VehicleType   string
	VehicleNumber string
}

// Config is the complete validated demo-user configuration.
type Config struct {
	Accounts []Account
}

// LoadConfig reads and validates the six demo accounts from the environment.
// It normalizes emails with the identity service's registration rules.
func LoadConfig(loader sharedconfig.Loader) (Config, error) {
	adminEmail, err := requiredEmail(loader, "SEED_ADMIN_EMAIL")
	if err != nil {
		return Config{}, err
	}
	adminPassword, err := requiredPassword(loader, "SEED_ADMIN_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	dispatcherEmail, err := requiredEmail(loader, "SEED_DISPATCHER_EMAIL")
	if err != nil {
		return Config{}, err
	}
	dispatcherPassword, err := requiredPassword(loader, "SEED_DISPATCHER_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	courierOneEmail, err := requiredEmail(loader, "SEED_COURIER_1_EMAIL")
	if err != nil {
		return Config{}, err
	}
	courierOnePassword, err := requiredPassword(loader, "SEED_COURIER_1_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	courierTwoEmail, err := requiredEmail(loader, "SEED_COURIER_2_EMAIL")
	if err != nil {
		return Config{}, err
	}
	courierTwoPassword, err := requiredPassword(loader, "SEED_COURIER_2_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	customerEmail, err := requiredEmail(loader, "SEED_CUSTOMER_EMAIL")
	if err != nil {
		return Config{}, err
	}
	customerPassword, err := requiredPassword(loader, "SEED_CUSTOMER_PASSWORD")
	if err != nil {
		return Config{}, err
	}

	config := Config{Accounts: []Account{
		{
			Role:     auth.RoleAdmin,
			Email:    adminEmail,
			Password: adminPassword,
			FullName: "Demo Admin",
		},
		{
			Role:     auth.RoleDispatcher,
			Email:    dispatcherEmail,
			Password: dispatcherPassword,
			FullName: "Demo Dispatcher",
		},
		{
			Role:          auth.RoleCourier,
			Email:         courierOneEmail,
			Password:      courierOnePassword,
			FullName:      "Demo Courier 1",
			HomeZoneCode:  courierOneHomeZone,
			VehicleType:   "BIKE",
			VehicleNumber: "SEED-BIKE-01",
		},
		{
			Role:          auth.RoleCourier,
			Email:         courierTwoEmail,
			Password:      courierTwoPassword,
			FullName:      "Demo Courier 2",
			HomeZoneCode:  courierTwoHomeZone,
			VehicleType:   "VAN",
			VehicleNumber: "SEED-VAN-01",
		},
		{
			Role:     auth.RoleCustomer,
			Email:    customerEmail,
			Password: customerPassword,
			FullName: "Demo Customer",
		},
		{
			Role:     auth.RoleMerchant,
			Email:    "seed.merchant@example.com",
			Password: "MerchantPass1!",
			FullName: "Demo Merchant",
		},
	}}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks the complete seed configuration before any database work.
func (c Config) Validate() error {
	if len(c.Accounts) != 6 {
		return errors.New("demo seed must contain exactly six accounts")
	}

	roleCounts := map[auth.Role]int{}
	emails := map[string]bool{}
	for index, account := range c.Accounts {
		if !account.Role.Valid() {
			return fmt.Errorf("account %d has an invalid role", index+1)
		}
		roleCounts[account.Role]++

		email := service.NormalizeEmail(account.Email)
		if !service.ValidEmail(email) {
			return fmt.Errorf("account %d has an invalid email", index+1)
		}
		if emails[email] {
			return errors.New("demo account emails must be unique")
		}
		emails[email] = true

		if err := password.Validate(account.Password); err != nil {
			return fmt.Errorf("account %d password: %w", index+1, err)
		}

		nameLength := validation.RuneLength(strings.TrimSpace(account.FullName))
		if nameLength < minFullNameLength || nameLength > maxFullNameLength {
			return fmt.Errorf("account %d full name must be 2 to %d characters", index+1, maxFullNameLength)
		}

		if account.Role == auth.RoleCourier {
			if strings.TrimSpace(account.HomeZoneCode) == "" {
				return fmt.Errorf("courier account %d home zone code is required", index+1)
			}
			if strings.TrimSpace(account.VehicleType) == "" || len(account.VehicleType) > maxVehicleTypeLength {
				return fmt.Errorf("courier account %d vehicle type is invalid", index+1)
			}
			if strings.TrimSpace(account.VehicleNumber) == "" || len(account.VehicleNumber) > maxVehicleNumberLen {
				return fmt.Errorf("courier account %d vehicle number is invalid", index+1)
			}
		}
	}

	if roleCounts[auth.RoleAdmin] != 1 ||
		roleCounts[auth.RoleDispatcher] != 1 ||
		roleCounts[auth.RoleCourier] != 2 ||
		roleCounts[auth.RoleCustomer] != 1 ||
		roleCounts[auth.RoleMerchant] != 1 {
		return errors.New("demo seed must contain one ADMIN, one DISPATCHER, two COURIER, one CUSTOMER and one MERCHANT account")
	}
	return nil
}

func requiredEmail(loader sharedconfig.Loader, key string) (string, error) {
	value, err := loader.Required(key)
	if err != nil {
		return "", err
	}
	email := service.NormalizeEmail(value)
	if !service.ValidEmail(email) {
		return "", fmt.Errorf("%s must be a valid email address", key)
	}
	return email, nil
}

func requiredPassword(loader sharedconfig.Loader, key string) (string, error) {
	value, err := loader.Required(key)
	if err != nil {
		return "", err
	}
	if err := password.Validate(value); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}
