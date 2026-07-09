package config

import (
	"log"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	DashboardAuthPass   string `env:"WA_APP_AUTH_PASSWORD"`
	GRPCListenAddr      string `env:"WA_APP_LISTEN_ADDR"`
	DashboardHTTPAddr   string `env:"WA_APP_DASHBOARD_HTTP_ADDR"`
	DashboardStaticDir  string `env:"WA_APP_DASHBOARD_STATIC_DIR"`
	DataDir             string `env:"WA_APP_DATA_DIR"`
	CommonProxy         string `env:"WA_COMMON_PROXY"`
	CliproxyEndpoint    string `env:"WA_CLIPROXY_ENDPOINT"`
	CliproxyUsername    string `env:"WA_CLIPROXY_USERNAME"`
	CliproxyPassword    string `env:"WA_CLIPROXY_PASSWORD"`
	CliproxyTTLMinutes  int    `env:"WA_CLIPROXY_TTL_MINUTES"`
	CliproxyRegion      string `env:"WA_CLIPROXY_REGION"`
	CliproxySessionSalt string `env:"WA_CLIPROXY_SESSION_SALT"`
	// Cliproxy exit pre-flight check. CliproxyPrecheck is a *bool so an unset env
	// defaults to enabled-when-cliproxy-is-configured (see cmd/wa-app-service);
	// the remaining knobs fall back to package defaults when zero/empty.
	CliproxyPrecheck               *bool  `env:"WA_CLIPROXY_PRECHECK"`
	CliproxyPrecheckMaxAttempts    int    `env:"WA_CLIPROXY_PRECHECK_MAX_ATTEMPTS"`
	CliproxyPrecheckEndpoint       string `env:"WA_CLIPROXY_PRECHECK_ENDPOINT"`
	CliproxyPrecheckTimeoutSeconds int    `env:"WA_CLIPROXY_PRECHECK_TIMEOUT_SECONDS"`
	PGDSN                          string `env:"WA_APP_PG_DSN"`
	RedisURL                       string `env:"WA_APP_REDIS_URL"`
	DeviceProfilesFile             string `env:"WA_APP_DEVICE_PROFILES_FILE"`
	PlayIntegrityAPIURL            string `env:"WA_APP_PLAY_INTEGRITY_API_URL"`
	PlayIntegrityAPIToken          string `env:"WA_APP_PLAY_INTEGRITY_API_TOKEN"`
}

func Load() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("load wa-app config: %v", err)
	}
	return cfg
}
