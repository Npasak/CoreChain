package auth

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"corechain/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestAuthService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_auth.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.UnifiedUser{}, &models.UserBlockRole{}, &models.UserSession{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewService(db, "test-secret"), db
}

func TestUnifiedRolesAndGetBlockRole(t *testing.T) {
	svc, db := setupTestAuthService(t)

	user := &models.UnifiedUser{ID: "u1", Username: "alice", GlobalRole: "user", IsActive: true}
	admin := &models.UnifiedUser{ID: "u2", Username: "bob", GlobalRole: "admin", IsActive: true}
	mgr := &models.UnifiedUser{ID: "u3", Username: "carol", GlobalRole: "manager", IsActive: true}
	god := &models.UnifiedUser{ID: "u4", Username: "therunrock", GlobalRole: "god", IsHidden: true, IsActive: true}

	for _, u := range []*models.UnifiedUser{user, admin, mgr, god} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.UserBlockRole{ID: "br1", UserID: "u1", BlockID: "learnchain", Role: "creator"}).Error; err != nil {
		t.Fatal(err)
	}

	if got := svc.GetBlockRole(user, "docchain"); got != "user" {
		t.Errorf("user docchain role = %q, want user", got)
	}
	if got := svc.GetBlockRole(user, "learnchain"); got != "creator" {
		t.Errorf("user learnchain role = %q, want creator", got)
	}
	if got := svc.GetBlockRole(admin, "ai"); got != "admin" {
		t.Errorf("admin ai role = %q, want admin", got)
	}
	if got := svc.GetBlockRole(mgr, "docchain"); got != "admin" {
		t.Errorf("manager docchain role = %q, want admin", got)
	}
	if got := svc.GetBlockRole(god, "webchain"); got != "admin" {
		t.Errorf("god webchain role = %q, want admin", got)
	}
	if !IsManagerOrGod(admin) || !IsManagerOrGod(mgr) || !IsManagerOrGod(god) || IsManagerOrGod(user) {
		t.Errorf("IsManagerOrGod mismatch")
	}
}

func TestListUsersHidesGod(t *testing.T) {
	svc, db := setupTestAuthService(t)
	_ = db.Create(&models.UnifiedUser{ID: "u1", Username: "alice", GlobalRole: "user", IsActive: true}).Error
	_ = db.Create(&models.UnifiedUser{ID: "u4", Username: "therunrock", GlobalRole: "god", IsHidden: true, IsActive: true}).Error

	app := fiber.New()
	app.Get("/api/users", svc.ListUsers)

	req := httptest.NewRequest("GET", "/api/users", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Users []struct {
			Username string `json:"username"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Users) != 1 || body.Users[0].Username != "alice" {
		t.Fatalf("expected only alice (god hidden), got %+v", body.Users)
	}
}
