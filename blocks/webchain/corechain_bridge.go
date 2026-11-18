package main

import (
	"fmt"
	"path/filepath"
	"time"

	"corechain/sdk/blocksdk"
	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func setupCoreChainBridge(app *fiber.App, env blocksdk.RuntimeEnv) {
	app.Use(blocksdk.UnifiedAuthMiddleware(env.BusSecret))

	// Bridge CoreChain Unified Auth user into WebChain's local c.Locals("user")
	app.Use(func(c *fiber.Ctx) error {
		if cu := blocksdk.GetCoreUser(c); cu != nil {
			var u models.User
			if err := database.DB.Where("username = ?", cu.Username).First(&u).Error; err != nil {
				role := "poster"
				if cu.GlobalRole == "admin" || cu.GlobalRole == "god" || cu.BlockRole == "admin" {
					role = cu.GlobalRole
					if role == "user" {
						role = "admin"
					}
				}
				u = models.User{
					ID:          uuid.New().String(),
					Username:    cu.Username,
					DisplayName: cu.DisplayName,
					Role:        role,
					Department:  cu.Department,
					Ward:        cu.Ward,
					IsActive:    true,
				}
				_ = database.DB.Create(&u).Error
			}
			c.Locals("user", &u)
			c.Locals("userID", u.ID)
			c.Locals("userRole", u.Role)
		}
		return c.Next()
	})

	blocksdk.RegisterStandardRoutes(app, blocksdk.StandardOptions{
		BlockID: env.BlockID,
		Version: "1.0.0",
		StatsProvider: func() blocksdk.BlockStats {
			var postCount, wardCount int64
			database.DB.Model(&models.Post{}).Count(&postCount)
			database.DB.Model(&models.WardProfile{}).Count(&wardCount)
			return blocksdk.BlockStats{
				BlockID: env.BlockID,
				Status:  "running",
				Metrics: map[string]any{"posts": postCount, "wards": wardCount},
				Summary: fmt.Sprintf("ประกาศ %d รายการ • หอผู้ป่วย %d แห่ง", postCount, wardCount),
			}
		},
		RPCHandlers: map[string]blocksdk.RPCHandler{
			"latest_posts": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				var posts []models.Post
				database.DB.Order("created_at desc").Limit(5).Find(&posts)
				return posts, nil
			},
		},
		EventHandler: func(topic, sender string, payload map[string]any) {
			if topic == "doc.published" {
				title, _ := payload["title"].(string)
				cat, _ := payload["category"].(string)
				if title == "" {
					return
				}
				post := models.Post{
					ID:          uuid.New().String(),
					Category:    "announcement",
					ScopeDept:   "all",
					TargetWards: `["all"]`,
					Title:       fmt.Sprintf("📄 เอกสารใหม่ใน DocChain: %s", title),
					Content:     fmt.Sprintf("หมวดหมู่: %s (เผยแพร่อัตโนมัติผ่าน CoreChain Connector Bus)", cat),
					Priority:    "normal",
					AuthorName:  "DocChain Connector",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				_ = database.DB.Create(&post).Error
			}
		},
	})

	// Unified Pic/webchain/ upload endpoint
	app.Post("/api/pic/upload", func(c *fiber.Ctx) error {
		file, err := c.FormFile("file")
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "missing file"})
		}
		ext := filepath.Ext(file.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		dest := filepath.Join(env.PicDir, name)
		if err := c.SaveFile(file, dest); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"url": blocksdk.PicURL(env.BlockID, name)})
	})
}
