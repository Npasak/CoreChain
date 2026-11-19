package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const Version = "1.2.4"

type Config struct {
	Port               string
	DataDir            string
	PicDir             string
	BlocksDir          string
	PackagesDir        string
	BackupsDir         string
	LegacyWebChainDB   string
	LegacyLearnChainDB string
	AutoUpdateEnabled  bool
	AutoUpdateMin      int
	UpdateManifestURL  string
	AuthSecret         string
	AutoOpenBrowser    bool
}

func Load() *Config {
	_ = godotenv.Load()

	dataDir := getEnv("CORECHAIN_DATA_DIR", "data")
	picDir := getEnv("CORECHAIN_PIC_DIR", "Pic")
	blocksDir := getEnv("CORECHAIN_BLOCKS_DIR", "installed_blocks")
	packagesDir := getEnv("CORECHAIN_PACKAGES_DIR", "packages")
	backupsDir := getEnv("CORECHAIN_BACKUPS_DIR", "backups")

	for _, d := range []string{dataDir, picDir, blocksDir, packagesDir, backupsDir} {
		_ = os.MkdirAll(d, 0755)
	}

	secret := getEnv("CORECHAIN_AUTH_SECRET", "")
	if secret == "" {
		secret = loadOrCreateSecret(filepath.Join(dataDir, ".secret"))
	}

	intervalMin, _ := strconv.Atoi(getEnv("AUTO_UPDATE_INTERVAL_MIN", "30"))
	if intervalMin <= 0 {
		intervalMin = 30
	}

	return &Config{
		Port:               getEnv("CORECHAIN_PORT", "80"),
		DataDir:            dataDir,
		PicDir:             picDir,
		BlocksDir:          blocksDir,
		PackagesDir:        packagesDir,
		BackupsDir:         backupsDir,
		LegacyWebChainDB:   getEnv("LEGACY_WEBCHAIN_DB", filepath.Join("..", "WebChain", "webchain.db")),
		LegacyLearnChainDB: getEnv("LEGACY_LEARNCHAIN_DB", filepath.Join("..", "DApps", "elearning.db")),
		AutoUpdateEnabled:  getEnvBool("AUTO_UPDATE_ENABLED", true),
		AutoUpdateMin:      intervalMin,
		UpdateManifestURL:  getEnv("UPDATE_MANIFEST_URL", "https://raw.githubusercontent.com/Npasak/CoreChain/main/manifest.json"),
		AuthSecret:         secret,
		AutoOpenBrowser:    getEnvBool("AUTO_OPEN_BROWSER", true),
	}
}

func loadOrCreateSecret(path string) string {
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 16 {
			return s
		}
	}
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	s := hex.EncodeToString(buf)
	_ = os.WriteFile(path, []byte(s), 0600)
	return s
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
}
