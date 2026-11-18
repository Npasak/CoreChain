package database

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"corechain/internal/config"
	"corechain/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MigrateLegacyData copies webchain.db & elearning.db into data/ if not yet present
// and imports users into CoreChain Unified Auth.
func MigrateLegacyData(coreDB *gorm.DB, cfg *config.Config) {
	webDest := filepath.Join(cfg.DataDir, "webchain.db")
	learnDest := filepath.Join(cfg.DataDir, "learnchain.db")

	copyIfMissing(cfg.LegacyWebChainDB, webDest, "WebChain")
	copyIfMissing(cfg.LegacyLearnChainDB, learnDest, "LearnChain")

	// Copy legacy LearnChain covers into Pic/learnchain if present
	legacyCovers := filepath.Join(filepath.Dir(cfg.LegacyLearnChainDB), "covers")
	picLearnDir := filepath.Join(cfg.PicDir, "learnchain")
	copyDirIfEmpty(legacyCovers, picLearnDir)

	var flag models.SystemSetting
	if err := coreDB.Where("key = ?", "legacy_users_migrated").First(&flag).Error; err == nil && flag.Value == "true" {
		return
	}

	imported := 0
	if fileExists(webDest) {
		imported += importWebChainUsers(coreDB, webDest)
	}
	if fileExists(learnDest) {
		imported += importLearnChainUsers(coreDB, learnDest)
	}

	coreDB.Save(&models.SystemSetting{
		Key:   "legacy_users_migrated",
		Value: "true",
	})
	if imported > 0 {
		log.Printf("[Migration] Imported %d legacy user records into Unified Auth", imported)
	}
}

type legacyWebUser struct {
	ID           string `gorm:"column:id"`
	Username     string `gorm:"column:username"`
	DisplayName  string `gorm:"column:display_name"`
	PasswordHash string `gorm:"column:password_hash"`
	Role         string `gorm:"column:role"`
	Department   string `gorm:"column:department"`
	Ward         string `gorm:"column:ward"`
	IsActive     bool   `gorm:"column:is_active"`
	IsHidden     bool   `gorm:"column:is_hidden"`
}

func (legacyWebUser) TableName() string { return "users" }

type legacyLearnUser struct {
	ID            uint   `gorm:"column:id"`
	WalletAddress string `gorm:"column:wallet_address"`
	Name          string `gorm:"column:name"`
	Role          string `gorm:"column:role"`
	Email         string `gorm:"column:email"`
	PasswordHash  string `gorm:"column:password_hash"`
	Ward          string `gorm:"column:ward"`
	AvatarURL     string `gorm:"column:avatar_url"`
}

func (legacyLearnUser) TableName() string { return "users" }

func importWebChainUsers(coreDB *gorm.DB, dbPath string) int {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return 0
	}
	var users []legacyWebUser
	if err := db.Find(&users).Error; err != nil {
		return 0
	}
	count := 0
	for _, lu := range users {
		uname := strings.TrimSpace(lu.Username)
		if uname == "" {
			continue
		}
		var existing models.UnifiedUser
		err := coreDB.Where("username = ?", uname).First(&existing).Error
		if err != nil {
			globalRole := "user"
			if lu.Role == "admin" || lu.Role == "god" {
				globalRole = lu.Role
			}
			u := models.UnifiedUser{
				ID:           uuid.New().String(),
				Username:     uname,
				DisplayName:  lu.DisplayName,
				PasswordHash: lu.PasswordHash,
				GlobalRole:   globalRole,
				Department:   lu.Department,
				Ward:         lu.Ward,
				IsActive:     lu.IsActive,
				IsHidden:     lu.IsHidden,
				LegacyWebID:  lu.ID,
			}
			if err := coreDB.Create(&u).Error; err == nil {
				existing = u
				count++
			}
		} else if existing.LegacyWebID == "" {
			coreDB.Model(&existing).Update("legacy_web_id", lu.ID)
		}

		if existing.ID != "" {
			upsertBlockRole(coreDB, existing.ID, "webchain", lu.Role)
		}
	}
	return count
}

func importLearnChainUsers(coreDB *gorm.DB, dbPath string) int {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return 0
	}
	var users []legacyLearnUser
	if err := db.Find(&users).Error; err != nil {
		return 0
	}
	count := 0
	for _, lu := range users {
		email := strings.TrimSpace(strings.ToLower(lu.Email))
		uname := email
		if uname == "" {
			uname = fmt.Sprintf("learn_%d", lu.ID)
		}
		var existing models.UnifiedUser
		q := coreDB.Where("username = ?", uname)
		if email != "" {
			q = q.Or("email = ?", email)
		}
		if lu.WalletAddress != "" {
			q = q.Or("wallet_address = ?", lu.WalletAddress)
		}
		err := q.First(&existing).Error
		if err != nil {
			pwHash := lu.PasswordHash
			if pwHash == "" {
				b, _ := bcrypt.GenerateFromPassword([]byte("learnchain123"), bcrypt.DefaultCost)
				pwHash = string(b)
			}
			displayName := lu.Name
			if displayName == "" {
				displayName = uname
			}
			u := models.UnifiedUser{
				ID:            uuid.New().String(),
				Username:      uname,
				Email:         email,
				DisplayName:   displayName,
				PasswordHash:  pwHash,
				GlobalRole:    "user",
				Ward:          lu.Ward,
				WalletAddress: lu.WalletAddress,
				AvatarURL:     lu.AvatarURL,
				IsActive:      true,
				LegacyLearnID: lu.ID,
			}
			if err := coreDB.Create(&u).Error; err == nil {
				existing = u
				count++
			}
		} else {
			updates := map[string]any{}
			if existing.LegacyLearnID == 0 {
				updates["legacy_learn_id"] = lu.ID
			}
			if existing.WalletAddress == "" && lu.WalletAddress != "" {
				updates["wallet_address"] = lu.WalletAddress
			}
			if len(updates) > 0 {
				coreDB.Model(&existing).Updates(updates)
			}
		}

		if existing.ID != "" {
			role := lu.Role
			if role == "" {
				role = "learner"
			}
			upsertBlockRole(coreDB, existing.ID, "learnchain", role)
		}
	}
	return count
}

func upsertBlockRole(db *gorm.DB, userID, blockID, role string) {
	if role == "" {
		return
	}
	var br models.UserBlockRole
	err := db.Where("user_id = ? AND block_id = ?", userID, blockID).First(&br).Error
	if err != nil {
		db.Create(&models.UserBlockRole{
			ID:      uuid.New().String(),
			UserID:  userID,
			BlockID: blockID,
			Role:    role,
		})
	}
}

func copyIfMissing(src, dst, label string) {
	if fileExists(dst) || !fileExists(src) {
		return
	}
	if err := copyFile(src, dst); err != nil {
		log.Printf("[Migration] Warning: failed to copy %s DB from %s: %v", label, src, err)
		return
	}
	log.Printf("[Migration] Imported legacy %s database -> %s", label, dst)
}

func copyDirIfEmpty(srcDir, dstDir string) {
	if !dirExists(srcDir) {
		return
	}
	_ = os.MkdirAll(dstDir, 0755)
	entries, err := os.ReadDir(dstDir)
	if err == nil && len(entries) > 0 {
		return
	}
	srcEntries, err := os.ReadDir(srcDir)
	if err != nil {
		return
	}
	for _, e := range srcEntries {
		if e.IsDir() {
			continue
		}
		_ = copyFile(filepath.Join(srcDir, e.Name()), filepath.Join(dstDir, e.Name()))
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
