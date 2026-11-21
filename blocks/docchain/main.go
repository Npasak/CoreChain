package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"corechain/sdk/blocksdk"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

//go:embed dist/*
var embedDist embed.FS

type Document struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	DocCode      string         `gorm:"index" json:"doc_code"`
	Title        string         `gorm:"not null" json:"title"`
	Category     string         `gorm:"index;default:'SOP'" json:"category"` // SOP, คู่มือ, แบบฟอร์ม, ระเบียบ
	Ward         string         `gorm:"index;default:'all'" json:"ward"`
	Version      string         `gorm:"default:'1.0'" json:"version"`
	Description  string         `gorm:"type:text" json:"description"`
	Tags         string         `json:"tags"`
	FileName     string         `json:"file_name"`
	FilePath     string         `json:"-"`
	FileSize     int64          `json:"file_size"`
	MimeType     string         `json:"mime_type"`
	ContentHash  string         `gorm:"index" json:"content_hash"`
	CoverURL     string         `json:"cover_url,omitempty"`
	UploaderID   string         `json:"uploader_id"`
	UploaderName string         `json:"uploader_name"`
	Downloads    int64          `gorm:"default:0" json:"downloads"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

var BlockVersion = "1.0.0"

func main() {
	env := blocksdk.LoadRuntimeEnv("docchain", "8083")
	db, err := blocksdk.OpenBlockDB(env.DBPath)
	if err != nil {
		log.Fatalf("[DocChain] DB open failed: %v", err)
	}
	if err := db.AutoMigrate(&Document{}); err != nil {
		log.Fatalf("[DocChain] AutoMigrate failed: %v", err)
	}
	seedDemoDocs(db)

	connector := blocksdk.NewConnectorClient(env)
	app := fiber.New(fiber.Config{
		AppName:   "DocChain Block",
		BodyLimit: 100 * 1024 * 1024,
	})
	app.Use(recover.New())
	app.Use(blocksdk.UnifiedAuthMiddleware(env.BusSecret))

	blocksdk.RegisterStandardRoutes(app, blocksdk.StandardOptions{
		BlockID: env.BlockID,
		Version: BlockVersion,
		StatsProvider: func() blocksdk.BlockStats {
			var count int64
			db.Model(&Document{}).Count(&count)
			return blocksdk.BlockStats{
				BlockID: env.BlockID,
				Status:  "running",
				Metrics: map[string]any{"documents": count},
				Summary: fmt.Sprintf("เอกสารทั้งหมด %d รายการ (SHA-256 Verified)", count),
			}
		},
		RPCHandlers: map[string]blocksdk.RPCHandler{
			"latest": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				var docs []Document
				db.Order("created_at desc").Limit(5).Find(&docs)
				return docs, nil
			},
			"search": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				q, _ := payload["q"].(string)
				var docs []Document
				like := "%" + strings.TrimSpace(q) + "%"
				db.Where("title LIKE ? OR doc_code LIKE ? OR tags LIKE ?", like, like, like).Order("created_at desc").Limit(20).Find(&docs)
				return docs, nil
			},
		},
	})

	api := app.Group("/api")
	api.Get("/me", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"user": blocksdk.GetCoreUser(c)})
	})

	api.Get("/docs", func(c *fiber.Ctx) error {
		q := strings.TrimSpace(c.Query("q"))
		cat := strings.TrimSpace(c.Query("category"))
		ward := strings.TrimSpace(c.Query("ward"))
		tx := db.Model(&Document{})
		if cat != "" && cat != "all" {
			tx = tx.Where("category = ?", cat)
		}
		if ward != "" && ward != "all" {
			tx = tx.Where("ward = ? OR ward = 'all'", ward)
		}
		if q != "" {
			like := "%" + q + "%"
			tx = tx.Where("title LIKE ? OR doc_code LIKE ? OR tags LIKE ? OR description LIKE ?", like, like, like, like)
		}
		var docs []Document
		tx.Order("created_at desc").Find(&docs)
		return c.JSON(fiber.Map{"documents": docs})
	})

	api.Post("/docs", func(c *fiber.Ctx) error {
		u := blocksdk.GetCoreUser(c)
		if u == nil {
			return c.Status(401).JSON(fiber.Map{"error": "กรุณาเข้าสู่ระบบที่ CoreChain ก่อนอัปโหลดเอกสาร"})
		}
		title := strings.TrimSpace(c.FormValue("title"))
		if title == "" {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุชื่อเอกสาร"})
		}
		doc := Document{
			ID:           uuid.New().String(),
			DocCode:      strings.TrimSpace(c.FormValue("doc_code")),
			Title:        title,
			Category:     defaultIfEmpty(c.FormValue("category"), "SOP"),
			Ward:         defaultIfEmpty(c.FormValue("ward"), "all"),
			Version:      defaultIfEmpty(c.FormValue("version"), "1.0"),
			Description:  c.FormValue("description"),
			Tags:         c.FormValue("tags"),
			UploaderID:   u.UserID,
			UploaderName: u.DisplayName,
		}

		if file, err := c.FormFile("file"); err == nil && file != nil {
			safeName := fmt.Sprintf("%s_%s", doc.ID[:8], filepath.Base(file.Filename))
			destPath := filepath.Join(env.DataDir, safeName)
			if err := c.SaveFile(file, destPath); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			hash, _ := sha256File(destPath)
			doc.FileName = file.Filename
			doc.FilePath = safeName
			doc.FileSize = file.Size
			doc.MimeType = file.Header.Get("Content-Type")
			doc.ContentHash = hash
		}

		// Save cover image into Pic/docchain/
		if cover, err := c.FormFile("cover"); err == nil && cover != nil {
			ext := filepath.Ext(cover.Filename)
			if ext == "" {
				ext = ".jpg"
			}
			coverName := fmt.Sprintf("%s%s", doc.ID[:8], ext)
			coverPath := filepath.Join(env.PicDir, coverName)
			if err := c.SaveFile(cover, coverPath); err == nil {
				doc.CoverURL = blocksdk.PicURL(env.BlockID, coverName)
			}
		}

		if err := db.Create(&doc).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// Notify other blocks via Connector Bus
		go func() {
			_ = connector.EmitEvent("doc.published", map[string]any{
				"doc_id":   doc.ID,
				"title":    doc.Title,
				"category": doc.Category,
				"ward":     doc.Ward,
				"hash":     doc.ContentHash,
			})
		}()

		return c.Status(201).JSON(fiber.Map{"document": doc})
	})

	api.Get("/docs/:id/file", func(c *fiber.Ctx) error {
		var doc Document
		if err := db.Where("id = ?", c.Params("id")).First(&doc).Error; err != nil || doc.FilePath == "" {
			return c.Status(404).JSON(fiber.Map{"error": "File not found"})
		}
		db.Model(&doc).UpdateColumn("downloads", gorm.Expr("downloads + 1"))
		fullPath := filepath.Join(env.DataDir, doc.FilePath)
		if c.Query("download") == "1" {
			return c.Download(fullPath, doc.FileName)
		}
		return c.SendFile(fullPath)
	})

	api.Delete("/docs/:id", func(c *fiber.Ctx) error {
		u := blocksdk.GetCoreUser(c)
		if u == nil {
			return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
		}
		var doc Document
		if err := db.Where("id = ?", c.Params("id")).First(&doc).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบเอกสาร"})
		}
		isAdmin := u.BlockRole == "admin" || u.BlockRole == "manager" || u.GlobalRole == "admin" || u.GlobalRole == "manager" || u.GlobalRole == "god"
		if !isAdmin && doc.UploaderID != u.UserID {
			return c.Status(403).JSON(fiber.Map{"error": "เฉพาะผู้จัดการหรือผู้อัปโหลดเท่านั้นที่ลบเอกสารนี้ได้"})
		}
		db.Delete(&doc)
		return c.JSON(fiber.Map{"ok": true})
	})

	distFS, _ := fs.Sub(embedDist, "dist")
	app.Use("/", filesystem.New(filesystem.Config{
		Root:   http.FS(distFS),
		Browse: false,
	}))

	log.Printf("[DocChain] Worker listening on 127.0.0.1:%s (DB: %s, Pic: %s)", env.Port, env.DBPath, env.PicDir)
	log.Fatal(app.Listen("127.0.0.1:" + env.Port))
}

func seedDemoDocs(db *gorm.DB) {
	var count int64
	db.Model(&Document{}).Count(&count)
	if count > 0 {
		return
	}
	demos := []Document{
		{
			ID:           uuid.New().String(),
			DocCode:      "SOP-NS-001",
			Title:        "แนวทางปฏิบัติการพยาบาลผู้ป่วยวิกฤตเบื้องต้น",
			Category:     "SOP",
			Ward:         "all",
			Version:      "2.0",
			Description:  "มาตรฐานการประเมินและดูแลผู้ป่วยแบบ Decentralized Reference",
			Tags:         "SOP, วิกฤต, พยาบาล",
			UploaderName: "CoreChain System",
			ContentHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			ID:           uuid.New().String(),
			DocCode:      "FRM-IC-012",
			Title:        "แบบฟอร์มเฝ้าระวังการติดเชื้อในหอผู้ป่วย",
			Category:     "แบบฟอร์ม",
			Ward:         "สว11",
			Version:      "1.2",
			Description:  "แบบฟอร์มบันทึกประจำวันสำหรับหอผู้ป่วยพิเศษ",
			Tags:         "IC, แบบฟอร์ม, สว11",
			UploaderName: "CoreChain System",
			ContentHash:  "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4",
		},
	}
	for _, d := range demos {
		db.Create(&d)
	}
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func defaultIfEmpty(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return strings.TrimSpace(v)
}
