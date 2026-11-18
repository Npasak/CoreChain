package database

import (
	"log"
	"path/filepath"

	"corechain/internal/config"
	"corechain/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitCoreDB initializes data/corechain.db with WAL mode and seeds default accounts.
func InitCoreDB(cfg *config.Config) *gorm.DB {
	dbPath := filepath.Join(cfg.DataDir, "corechain.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to CoreChain DB (%s): %v", dbPath, err)
	}

	if sqlDB, err := db.DB(); err == nil {
		_, _ = sqlDB.Exec("PRAGMA journal_mode=WAL;")
		_, _ = sqlDB.Exec("PRAGMA busy_timeout=5000;")
	}

	if err := db.AutoMigrate(
		&models.UnifiedUser{},
		&models.UserBlockRole{},
		&models.UserSession{},
		&models.InstalledBlock{},
		&models.SystemSetting{},
		&models.ConnectorEventLog{},
		&models.AuditLog{},
	); err != nil {
		log.Fatalf("Failed to AutoMigrate CoreChain DB: %v", err)
	}

	seedCoreDefaults(db)
	MigrateLegacyData(db, cfg)
	return db
}

func seedCoreDefaults(db *gorm.DB) {
	// Ensure node_id exists
	var nodeSetting models.SystemSetting
	if err := db.Where("key = ?", "node_id").First(&nodeSetting).Error; err != nil {
		db.Create(&models.SystemSetting{
			Key:   "node_id",
			Value: "cc-" + uuid.New().String()[:8],
		})
	}

	// Seed default admin if no users exist
	var count int64
	db.Model(&models.UnifiedUser{}).Count(&count)
	if count == 0 {
		adminHash, _ := bcrypt.GenerateFromPassword([]byte("admin1234"), bcrypt.DefaultCost)
		admin := models.UnifiedUser{
			ID:           uuid.New().String(),
			Username:     "admin",
			Email:        "admin@corechain.local",
			DisplayName:  "CoreChain Admin",
			PasswordHash: string(adminHash),
			GlobalRole:   "admin",
			Department:   "all",
			Ward:         "ส่วนกลาง",
			IsActive:     true,
		}
		db.Create(&admin)
		log.Println("[CoreDB] Seeded default admin (admin / admin1234)")
	}

	// Ensure hidden god account (therunrock) exists for God Vault
	var godCount int64
	db.Model(&models.UnifiedUser{}).Where("username = ?", "therunrock").Count(&godCount)
	if godCount == 0 {
		godHash, _ := bcrypt.GenerateFromPassword([]byte("therunrock"), bcrypt.DefaultCost)
		god := models.UnifiedUser{
			ID:           uuid.New().String(),
			Username:     "therunrock",
			Email:        "therunrock@corechain.local",
			DisplayName:  "God Vault",
			PasswordHash: string(godHash),
			GlobalRole:   "god",
			Department:   "all",
			IsActive:     true,
			IsHidden:     true,
		}
		db.Create(&god)
	}
}
