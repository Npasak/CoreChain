package blocksdk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// RuntimeEnv holds environment configuration injected by CoreChain Supervisor.
type RuntimeEnv struct {
	BlockID    string
	Port       string
	DBPath     string
	PicDir     string
	BusURL     string
	BusSecret  string
	CPUThreads int
	DataDir    string
}

// LoadRuntimeEnv reads CoreChain worker environment variables with sensible standalone defaults.
func LoadRuntimeEnv(defaultBlockID, defaultPort string) RuntimeEnv {
	blockID := getEnv("CORECHAIN_BLOCK_ID", defaultBlockID)
	cpuThreads, _ := strconv.Atoi(getEnv("CORECHAIN_CPU_THREADS", "0"))
	if cpuThreads > 0 {
		runtime.GOMAXPROCS(cpuThreads)
	}

	picDir := getEnv("CORECHAIN_PIC_DIR", filepath.Join("Pic", blockID))
	_ = os.MkdirAll(picDir, 0755)

	dbPath := getEnv("CORECHAIN_DB_PATH", filepath.Join("data", blockID+".db"))
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)

	dataDir := getEnv("CORECHAIN_DATA_DIR", filepath.Join("data", blockID+"_files"))
	_ = os.MkdirAll(dataDir, 0755)

	return RuntimeEnv{
		BlockID:    blockID,
		Port:       getEnv("CORECHAIN_PORT", defaultPort),
		DBPath:     dbPath,
		PicDir:     picDir,
		BusURL:     getEnv("CORECHAIN_BUS_URL", "http://127.0.0.1:8080"),
		BusSecret:  getEnv("CORECHAIN_BUS_SECRET", "corechain-dev-secret"),
		CPUThreads: cpuThreads,
		DataDir:    dataDir,
	}
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// OpenBlockDB opens an isolated SQLite database for a block with WAL mode enabled.
func OpenBlockDB(dbPath string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err == nil {
		_, _ = sqlDB.Exec("PRAGMA journal_mode=WAL;")
		_, _ = sqlDB.Exec("PRAGMA busy_timeout=5000;")
		_, _ = sqlDB.Exec("PRAGMA foreign_keys=ON;")
	}
	return db, nil
}

// CoreUser represents the unified user identity passed from CoreChain Gateway.
type CoreUser struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	GlobalRole  string `json:"global_role"`
	BlockRole   string `json:"block_role"`
	Ward        string `json:"ward"`
	Department  string `json:"department"`
	WalletAddr  string `json:"wallet_address"`
}

// SignCoreUser creates an HMAC signature for gateway-to-block identity headers.
func SignCoreUser(secret string, u CoreUser) string {
	payload := strings.Join([]string{
		u.UserID, u.Username, u.DisplayName, u.Email,
		u.GlobalRole, u.BlockRole, u.Ward, u.Department, u.WalletAddr,
	}, "|")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// UnifiedAuthMiddleware extracts and verifies CoreChain's signed user headers.
func UnifiedAuthMiddleware(busSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid := c.Get("X-Core-User-ID")
		sig := c.Get("X-Core-Sig")
		if uid != "" && sig != "" {
			u := CoreUser{
				UserID:      uid,
				Username:    c.Get("X-Core-Username"),
				DisplayName: c.Get("X-Core-Display-Name"),
				Email:       c.Get("X-Core-Email"),
				GlobalRole:  c.Get("X-Core-Global-Role"),
				BlockRole:   c.Get("X-Core-Block-Role"),
				Ward:        c.Get("X-Core-Ward"),
				Department:  c.Get("X-Core-Dept"),
				WalletAddr:  c.Get("X-Core-Wallet"),
			}
			expected := SignCoreUser(busSecret, u)
			if hmac.Equal([]byte(sig), []byte(expected)) {
				c.Locals("core_user", &u)
			} else {
				log.Printf("[blocksdk] invalid gateway signature for user %s", uid)
			}
		}
		return c.Next()
	}
}

// GetCoreUser returns the verified CoreUser from Fiber context, or nil if unauthenticated.
func GetCoreUser(c *fiber.Ctx) *CoreUser {
	if v := c.Locals("core_user"); v != nil {
		if u, ok := v.(*CoreUser); ok {
			return u
		}
	}
	return nil
}

// PicURL returns the canonical public URL for a file saved in Pic/<block_id>/<filename>.
func PicURL(blockID, filename string) string {
	return fmt.Sprintf("/Pic/%s/%s", blockID, filename)
}

// BlockStats is returned by each block's /_internal/stats endpoint for the CoreChain Launcher cards.
type BlockStats struct {
	BlockID string            `json:"block_id"`
	Status  string            `json:"status"`
	Metrics map[string]any    `json:"metrics"`
	Summary string            `json:"summary"`
}

// RPCHandler handles an incoming Connector Bus RPC call from another block.
type RPCHandler func(c *fiber.Ctx, payload map[string]any) (any, error)

// EventHandler handles an incoming Pub/Sub event from the Connector Bus.
type EventHandler func(topic string, senderBlock string, payload map[string]any)

// StandardOptions configures standard internal endpoints for a CoreChain block.
type StandardOptions struct {
	BlockID       string
	Version       string
	StatsProvider func() BlockStats
	RPCHandlers   map[string]RPCHandler
	EventHandler  EventHandler
}

// RegisterStandardRoutes mounts /_internal/health, /_internal/stats, /_internal/rpc/:action, and /_internal/events.
func RegisterStandardRoutes(app *fiber.App, opts StandardOptions) {
	internal := app.Group("/_internal")

	internal.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"block_id":    opts.BlockID,
			"version":     opts.Version,
			"gomaxprocs":  runtime.GOMAXPROCS(0),
			"num_cpu":     runtime.NumCPU(),
			"goroutines":  runtime.NumGoroutine(),
		})
	})

	internal.Get("/stats", func(c *fiber.Ctx) error {
		if opts.StatsProvider != nil {
			return c.JSON(opts.StatsProvider())
		}
		return c.JSON(BlockStats{
			BlockID: opts.BlockID,
			Status:  "running",
			Metrics: map[string]any{},
		})
	})

	internal.Post("/rpc/:action", func(c *fiber.Ctx) error {
		action := c.Params("action")
		if opts.RPCHandlers == nil {
			return c.Status(404).JSON(fiber.Map{"error": "no rpc handlers registered"})
		}
		handler, ok := opts.RPCHandlers[action]
		if !ok {
			return c.Status(404).JSON(fiber.Map{"error": fmt.Sprintf("unknown rpc action: %s", action)})
		}
		var payload map[string]any
		_ = c.BodyParser(&payload)
		if payload == nil {
			payload = map[string]any{}
		}
		res, err := handler(c, payload)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "data": res})
	})

	internal.Post("/events", func(c *fiber.Ctx) error {
		var body struct {
			Topic       string         `json:"topic"`
			SenderBlock string         `json:"sender_block"`
			Payload     map[string]any `json:"payload"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid event payload"})
		}
		if opts.EventHandler != nil {
			go opts.EventHandler(body.Topic, body.SenderBlock, body.Payload)
		}
		return c.JSON(fiber.Map{"received": true})
	})
}
