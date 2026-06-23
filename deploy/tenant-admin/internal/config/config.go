package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr              string
	AdminSecret             string
	DataDir                 string
	KonnectAPIURL           string
	KonnectPlatformToken    string
	KonnectIngestRole       string
	OpenMeterURL            string
	OpenMeterDefaultPlanKey string
	SpatTTL                 time.Duration
	Auth0Domain             string
	Auth0MgmtClientID       string
	Auth0MgmtClientSecret   string
	Auth0DefaultConnection  string
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:              envDefault("TENANT_ADMIN_LISTEN_ADDR", ":8093"),
		DataDir:                 envDefault("TENANT_ADMIN_DATA_DIR", "./data"),
		KonnectAPIURL:           envDefault("KONNECT_API_URL", "https://global.api.konghq.com"),
		KonnectIngestRole:       envDefault("KONNECT_INGEST_ROLE", "Ingest"),
		OpenMeterURL:            envDefault("OPENMETER_URL", "https://us.api.konghq.com/v3/openmeter"),
		OpenMeterDefaultPlanKey: envDefault("OPENMETER_DEFAULT_PLAN_KEY", "clearinghouse_default_ppu"),
		Auth0DefaultConnection:  envDefault("AUTH0_DEFAULT_CONNECTION", "Username-Password-Authentication"),
	}

	cfg.AdminSecret = strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
	cfg.KonnectPlatformToken = strings.TrimSpace(os.Getenv("KONNECT_PLATFORM_PAT"))
	cfg.Auth0Domain = strings.TrimSpace(os.Getenv("AUTH0_MGMT_DOMAIN"))
	cfg.Auth0MgmtClientID = strings.TrimSpace(os.Getenv("AUTH0_MGMT_CLIENT_ID"))
	cfg.Auth0MgmtClientSecret = strings.TrimSpace(os.Getenv("AUTH0_MGMT_CLIENT_SECRET"))

	daysRaw := strings.TrimSpace(envDefault("KONNECT_SPAT_TTL_DAYS", "365"))
	days, err := strconv.Atoi(daysRaw)
	if err != nil || days < 1 {
		return Config{}, fmt.Errorf("KONNECT_SPAT_TTL_DAYS must be a positive integer")
	}
	cfg.SpatTTL = time.Duration(days) * 24 * time.Hour

	if cfg.AdminSecret == "" {
		return Config{}, fmt.Errorf("ADMIN_SECRET is required")
	}
	if cfg.KonnectPlatformToken == "" {
		return Config{}, fmt.Errorf("KONNECT_PLATFORM_PAT is required")
	}
	if cfg.Auth0Domain == "" {
		return Config{}, fmt.Errorf("AUTH0_MGMT_DOMAIN is required")
	}
	if cfg.Auth0MgmtClientID == "" {
		return Config{}, fmt.Errorf("AUTH0_MGMT_CLIENT_ID is required")
	}
	if cfg.Auth0MgmtClientSecret == "" {
		return Config{}, fmt.Errorf("AUTH0_MGMT_CLIENT_SECRET is required")
	}

	return cfg, nil
}

func envDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
