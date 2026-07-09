package config

import (
	"log"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	DashboardAuthPass    string `env:"WA_APP_AUTH_PASSWORD"`
	GRPCListenAddr       string `env:"WA_APP_LISTEN_ADDR"`
	DashboardHTTPAddr    string `env:"WA_APP_DASHBOARD_HTTP_ADDR"`
	DashboardStaticDir   string `env:"WA_APP_DASHBOARD_STATIC_DIR"`
	DataDir              string `env:"WA_APP_DATA_DIR"`
	CommonProxy          string `env:"WA_COMMON_PROXY"`
	BoltproxyEndpoint    string `env:"WA_BOLTPROXY_ENDPOINT"`
	BoltproxyUsername    string `env:"WA_BOLTPROXY_USERNAME"`
	BoltproxyPassword    string `env:"WA_BOLTPROXY_PASSWORD"`
	BoltproxyTTLMinutes  int    `env:"WA_BOLTPROXY_TTL_MINUTES"`
	BoltproxyRegion      string `env:"WA_BOLTPROXY_REGION"`
	BoltproxySessionSalt string `env:"WA_BOLTPROXY_SESSION_SALT"`
	// Boltproxy exit pre-flight check. BoltproxyPrecheck is a *bool so an unset env
	// defaults to enabled-when-boltproxy-is-configured (see cmd/wa-app-service);
	// the remaining knobs fall back to package defaults when zero/empty.
	BoltproxyPrecheck               *bool  `env:"WA_BOLTPROXY_PRECHECK"`
	BoltproxyPrecheckMaxAttempts    int    `env:"WA_BOLTPROXY_PRECHECK_MAX_ATTEMPTS"`
	BoltproxyPrecheckEndpoint       string `env:"WA_BOLTPROXY_PRECHECK_ENDPOINT"`
	BoltproxyPrecheckTimeoutSeconds int    `env:"WA_BOLTPROXY_PRECHECK_TIMEOUT_SECONDS"`
	PGDSN                           string `env:"WA_APP_PG_DSN"`
	RedisURL                        string `env:"WA_APP_REDIS_URL"`
	DeviceProfilesFile              string `env:"WA_APP_DEVICE_PROFILES_FILE"`
	PlayIntegrityAPIURL             string `env:"WA_APP_PLAY_INTEGRITY_API_URL"`
	PlayIntegrityAPIToken           string `env:"WA_APP_PLAY_INTEGRITY_API_TOKEN"`
}

func Load() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("load wa-app config: %v", err)
	}
	return cfg
}
