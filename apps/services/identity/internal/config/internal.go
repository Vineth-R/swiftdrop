package config

import (
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// MinInternalTokenLength is kept for callers that need the development
// minimum. Validation itself lives in the shared config package so identity
// and pricing apply identical rules.
const MinInternalTokenLength = sharedconfig.MinInternalTokenLength

// LoadInternalToken reads and validates INTERNAL_TOKEN for appEnv.
func LoadInternalToken(loader sharedconfig.Loader, appEnv string) (token, problem string) {
	return sharedconfig.LoadInternalToken(loader, appEnv)
}
