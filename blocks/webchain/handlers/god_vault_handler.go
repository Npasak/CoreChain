package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"webchain/config"
	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type GodVaultHandler struct{}

func NewGodVaultHandler() *GodVaultHandler {
	return &GodVaultHandler{}
}

type BackupSnapshot struct {
	App           string                 `json:"app"`
	CreatedAt     time.Time              `json:"created_at"`
	CreatedBy     string                 `json:"created_by"`
	Users         []models.User          `json:"users"`
	Posts         []models.Post          `json:"posts"`
	ExternalLinks []models.ExternalLink  `json:"external_links"`
	Settings      []models.SystemSetting `json:"settings"`
	AuditLogs     []models.AuditLog      `json:"audit_logs"`
}

func (h *GodVaultHandler) GetVaultStatus(c *fiber.Ctx) error {
	var totalPosts, activePosts, totalUsers, totalLogs int64
	database.DB.Unscoped().Model(&models.Post{}).Count(&totalPosts)
	database.DB.Model(&models.Post{}).Count(&activePosts)
	database.DB.Model(&models.User{}).Where("is_hidden = ?", false).Count(&totalUsers)
	database.DB.Model(&models.AuditLog{}).Count(&totalLogs)

	var dbSizeBytes int64
	dbPath := "webchain.db"
	if config.AppConfig != nil && config.AppConfig.DBPath != "" {
		dbPath = config.AppConfig.DBPath
	}
	if fi, err := os.Stat(dbPath); err == nil {
		dbSizeBytes = fi.Size()
	}

	var recentLogs []models.AuditLog
	database.DB.Order("created_at desc").Limit(30).Find(&recentLogs)

	backups := listBackupFiles()

	return c.JSON(fiber.Map{
		"success":       true,
		"total_posts":   totalPosts,
		"active_posts":  activePosts,
		"deleted_posts": totalPosts - activePosts,
		"total_users":   totalUsers,
		"total_logs":    totalLogs,
		"db_size_kb":    float64(dbSizeBytes) / 1024.0,
		"recent_logs":   recentLogs,
		"backups":       backups,
	})
}

func listBackupFiles() []fiber.Map {
	_ = os.MkdirAll("backups", 0755)
	entries, err := os.ReadDir("backups")
	if err != nil {
		return []fiber.Map{}
	}
	var list []fiber.Map
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, _ := e.Info()
		sizeKB := 0.0
		modTime := ""
		if info != nil {
			sizeKB = float64(info.Size()) / 1024.0
			modTime = info.ModTime().Format("2006-01-02 15:04:05")
		}
		list = append(list, fiber.Map{
			"filename":   e.Name(),
			"size_kb":    sizeKB,
			"created_at": modTime,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i]["filename"].(string) > list[j]["filename"].(string)
	})
	return list
}

func createSnapshotFile(reason string) (string, *BackupSnapshot, error) {
	_ = os.MkdirAll("backups", 0755)

	var users []models.User
	var posts []models.Post
	var links []models.ExternalLink
	var settings []models.SystemSetting
	var logs []models.AuditLog

	database.DB.Unscoped().Find(&users)
	database.DB.Unscoped().Find(&posts)
	database.DB.Find(&links)
	database.DB.Find(&settings)
	database.DB.Order("created_at desc").Limit(500).Find(&logs)

	snap := &BackupSnapshot{
		App:           "WebChain",
		CreatedAt:     time.Now(),
		CreatedBy:     "therunrock (" + reason + ")",
		Users:         users,
		Posts:         posts,
		ExternalLinks: links,
		Settings:      settings,
		AuditLogs:     logs,
	}

	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", nil, err
	}
	filename := fmt.Sprintf("webchain-backup-%s.json", time.Now().Format("20060102-150405"))
	fullPath := filepath.Join("backups", filename)
	if err := os.WriteFile(fullPath, raw, 0644); err != nil {
		return "", nil, err
	}
	return filename, snap, nil
}

func (h *GodVaultHandler) CreateBackup(c *fiber.Ctx) error {
	filename, _, err := createSnapshotFile("manual")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "สร้างไฟล์สำรองไม่สำเร็จ: " + err.Error()})
	}
	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: "therunrock",
		Action:    "backup",
		Target:    filename,
		Detail:    "สำรองข้อมูลระบบ WebChain",
		CreatedAt: time.Now(),
	})
	return c.JSON(fiber.Map{
		"success":  true,
		"filename": filename,
		"message":  "สำรองข้อมูลเรียบร้อย: " + filename,
	})
}

func (h *GodVaultHandler) DownloadBackup(c *fiber.Ctx) error {
	filename := filepath.Base(c.Params("filename"))
	if !strings.HasSuffix(filename, ".json") {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ชื่อไฟล์ไม่ถูกต้อง"})
	}
	fullPath := filepath.Join("backups", filename)
	if _, err := os.Stat(fullPath); err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบไฟล์สำรอง"})
	}
	return c.Download(fullPath, filename)
}

func (h *GodVaultHandler) RestoreBackup(c *fiber.Ctx) error {
	var body struct {
		Filename string          `json:"filename"`
		Snapshot *BackupSnapshot `json:"snapshot"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}

	var snap BackupSnapshot
	if body.Snapshot != nil {
		snap = *body.Snapshot
	} else if body.Filename != "" {
		safeName := filepath.Base(body.Filename)
		raw, err := os.ReadFile(filepath.Join("backups", safeName))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบไฟล์สำรอง"})
		}
		if err := json.Unmarshal(raw, &snap); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "ไฟล์สำรองเสียหาย"})
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณาระบุไฟล์สำรอง"})
	}

	// Safety backup before restore
	_, _, _ = createSnapshotFile("pre-restore")

	for _, u := range snap.Users {
		database.DB.Unscoped().Save(&u)
	}
	for _, p := range snap.Posts {
		database.DB.Unscoped().Save(&p)
	}
	for _, l := range snap.ExternalLinks {
		database.DB.Save(&l)
	}
	for _, s := range snap.Settings {
		database.DB.Save(&s)
	}

	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: "therunrock",
		Action:    "restore",
		Target:    body.Filename,
		Detail:    fmt.Sprintf("กู้คืนโพสต์ %d รายการ และผู้ใช้ %d บัญชี", len(snap.Posts), len(snap.Users)),
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("กู้คืนข้อมูลสำเร็จ (โพสต์ %d รายการ, ผู้ใช้ %d บัญชี)", len(snap.Posts), len(snap.Users)),
	})
}

// ResetSystem (Oblivion Sweep): backs up first, then clears posts, audit logs, and optionally non-god users
func (h *GodVaultHandler) ResetSystem(c *fiber.Ctx) error {
	var body struct {
		IncludeUsers bool   `json:"include_users"`
		ConfirmText  string `json:"confirm_text"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}
	if strings.TrimSpace(body.ConfirmText) != "RESET" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณาพิมพ์ RESET เพื่อยืนยันการล้างระบบ"})
	}

	backupName, _, err := createSnapshotFile("pre-reset")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "สำรองข้อมูลก่อนล้างระบบไม่สำเร็จ"})
	}

	database.DB.Unscoped().Where("1 = 1").Delete(&models.Post{})
	database.DB.Unscoped().Where("1 = 1").Delete(&models.AuditLog{})
	if body.IncludeUsers {
		database.DB.Unscoped().Where("is_hidden = ?", false).Delete(&models.User{})
	}
	database.DB.Save(&models.SystemSetting{
		Key:       "initial_seeded",
		Value:     "true",
		UpdatedAt: time.Now(),
	})

	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: "therunrock",
		Action:    "reset_system",
		Target:    "all",
		Detail:    "ล้างระบบเรียบร้อย (สำรองไว้ที่ " + backupName + ")",
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{
		"success":     true,
		"backup_file": backupName,
		"message":     "ล้างระบบเรียบร้อยแล้ว (สำรองข้อมูลอัตโนมัติที่ " + backupName + ")",
	})
}
