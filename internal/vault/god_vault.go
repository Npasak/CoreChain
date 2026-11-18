package vault

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"corechain/internal/config"
	"corechain/internal/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// GodVault handles unified backup of all isolated block DBs + Pic/ media.
type GodVault struct {
	cfg *config.Config
	db  *gorm.DB
}

func NewGodVault(cfg *config.Config, db *gorm.DB) *GodVault {
	return &GodVault{cfg: cfg, db: db}
}

func (g *GodVault) Status(c *fiber.Ctx) error {
	var dbFiles []map[string]any
	entries, _ := os.ReadDir(g.cfg.DataDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
			info, _ := e.Info()
			var sz int64
			if info != nil {
				sz = info.Size()
			}
			dbFiles = append(dbFiles, map[string]any{
				"name": e.Name(),
				"size": sz,
			})
		}
	}
	var backups []string
	bEntries, _ := os.ReadDir(g.cfg.BackupsDir)
	for _, e := range bEntries {
		if !e.IsDir() {
			backups = append(backups, e.Name())
		}
	}
	return c.JSON(fiber.Map{
		"databases": dbFiles,
		"backups":   backups,
	})
}

func (g *GodVault) CreateBackup(c *fiber.Ctx) error {
	name := fmt.Sprintf("corechain_vault_%s.zip", time.Now().Format("20060102_150405"))
	outPath := filepath.Join(g.cfg.BackupsDir, name)

	f, err := os.Create(outPath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	addDirToZip(zw, g.cfg.DataDir, "data")
	addDirToZip(zw, g.cfg.PicDir, "Pic")
	_ = zw.Close()

	g.db.Create(&models.AuditLog{
		ID:        fmt.Sprintf("vault_%d", time.Now().UnixNano()),
		Action:    "god_backup",
		Target:    name,
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{
		"ok":       true,
		"filename": name,
		"download": "/api/god/backup/" + name,
	})
}

func (g *GodVault) DownloadBackup(c *fiber.Ctx) error {
	name := filepath.Base(c.Params("filename"))
	p := filepath.Join(g.cfg.BackupsDir, name)
	if _, err := os.Stat(p); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Backup not found"})
	}
	return c.Download(p, name)
}

func addDirToZip(zw *zip.Writer, srcDir, prefix string) {
	_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return nil
		}
		zipPath := filepath.ToSlash(filepath.Join(prefix, rel))
		w, err := zw.Create(zipPath)
		if err != nil {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer in.Close()
		_, _ = io.Copy(w, in)
		return nil
	})
}
