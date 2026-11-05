package config

// LoadAppEnv reads APP_ENV and requires it to be set explicitly to one of the
// supported deployment environments. Missing, empty, and whitespace-only values
// fail; there is no development fallback.
func LoadAppEnv(loader Loader) (string, error) {
	if _, err := loader.Required("APP_ENV"); err != nil {
		return "", err
	}
	return loader.EnumFold("APP_ENV", "", "development", "staging", "production")
}
