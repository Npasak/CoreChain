package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	DBPath          string
	AutoOpenBrowser bool

	// Connected systems
	LearnChainURL   string
	DocSearchURL    string
	DocSearchStatus string

	// Sync & Auto-Update
	GasSyncURL       string
	UpdateRepo       string
	TherunrockSecret string
}

var AppConfig *Config

func LoadConfig() *Config {
	_ = godotenv.Load()

	autoOpenStr := getEnv("AUTO_OPEN_BROWSER", "true")
	autoOpen, _ := strconv.ParseBool(autoOpenStr)

	AppConfig = &Config{
		Port:             getEnv("PORT", "8889"),
		DBPath:           getEnv("DB_PATH", "webchain.db"),
		AutoOpenBrowser:  autoOpen,
		LearnChainURL:    getEnv("LEARNCHAIN_URL", "http://localhost:8888"),
		DocSearchURL:     getEnv("DOCSEARCH_URL", ""),
		DocSearchStatus:  getEnv("DOCSEARCH_STATUS", "coming_soon"),
		GasSyncURL:       getEnv("GAS_SYNC_URL", ""),
		UpdateRepo:       getEnv("UPDATE_REPO", "Npasak/webchain"),
		TherunrockSecret: getEnv("THERUNROCK_SECRET", "therunrock2026"),
	}
	return AppConfig
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
