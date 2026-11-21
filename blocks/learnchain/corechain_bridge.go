package main

import (
	"fmt"

	"corechain/sdk/blocksdk"
	"elearning-sbt/database"
	"elearning-sbt/handlers"
	"elearning-sbt/models"

	"github.com/gofiber/fiber/v2"
)

func setupCoreChainBridge(app *fiber.App, env blocksdk.RuntimeEnv) {
	// Normalize any /Pic/learnchain/ URLs to /covers/ so standalone LearnChain.exe peers can fetch them from GAS/Peers
	database.DB.Exec("UPDATE lessons SET thumbnail_url = REPLACE(thumbnail_url, '/Pic/learnchain/', '/covers/') WHERE thumbnail_url LIKE '/Pic/learnchain/%'")
	database.DB.Exec("UPDATE courses SET thumbnail_url = REPLACE(thumbnail_url, '/Pic/learnchain/', '/covers/') WHERE thumbnail_url LIKE '/Pic/learnchain/%'")
	database.DB.Exec("UPDATE signatories SET signature_url = REPLACE(signature_url, '/Pic/learnchain/', '/covers/') WHERE signature_url LIKE '/Pic/learnchain/%'")

	app.Use(blocksdk.UnifiedAuthMiddleware(env.BusSecret))

	// Bridge CoreChain Unified Auth user into LearnChain's X-User-ID / Locals
	app.Use(func(c *fiber.Ctx) error {
		if cu := blocksdk.GetCoreUser(c); cu != nil {
			role := "learner"
			if cu.GlobalRole == "admin" || cu.GlobalRole == "manager" || cu.GlobalRole == "god" || cu.BlockRole == "admin" || cu.BlockRole == "manager" {
				role = "manager"
			} else if cu.BlockRole == "creator" {
				role = "creator"
			}

			var u models.User
			err := database.DB.Where("LOWER(email) = ? OR wallet_address = ?", cu.Email, cu.WalletAddr).First(&u).Error
			if err != nil && cu.Username != "" {
				err = database.DB.Where("LOWER(email) = ?", cu.Username).First(&u).Error
			}
			if err != nil {
				wallet := cu.WalletAddr
				if wallet == "" {
					idPrefix := cu.UserID
					if len(idPrefix) > 8 {
						idPrefix = idPrefix[:8]
					}
					wallet = "cc-" + idPrefix
				}
				email := cu.Email
				if email == "" {
					email = cu.Username + "@corechain.local"
				}
				u = models.User{
					WalletAddress: wallet,
					Name:          cu.DisplayName,
					Role:          role,
					Email:         email,
					Ward:          cu.Ward,
				}
				_ = database.DB.Create(&u).Error
			} else if cu.GlobalRole != "god" && cu.Username != "therunrock" && (u.Role != role || (cu.DisplayName != "" && u.Name != cu.DisplayName) || u.Ward != cu.Ward) {
				u.Role = role
				if cu.DisplayName != "" {
					u.Name = cu.DisplayName
				}
				u.Ward = cu.Ward
				_ = database.DB.Model(&u).Updates(map[string]any{
					"role": u.Role,
					"name": u.Name,
					"ward": u.Ward,
				}).Error
			}
			if cu.GlobalRole == "god" || cu.Username == "therunrock" {
				c.Request().Header.Set("X-User-ID", fmt.Sprintf("%d", handlers.SecretAdminUserID))
				c.Locals("user", &u)
			} else if u.ID > 0 {
				c.Request().Header.Set("X-User-ID", fmt.Sprintf("%d", u.ID))
				c.Locals("user", &u)
			}
		}
		return c.Next()
	})

	// Serve Pic/learnchain/ for covers & signatures
	app.Static("/covers", env.PicDir)

	blocksdk.RegisterStandardRoutes(app, blocksdk.StandardOptions{
		BlockID: env.BlockID,
		Version: "v1.3.22",
		StatsProvider: func() blocksdk.BlockStats {
			var courses, lessons, certs int64
			database.DB.Model(&models.Course{}).Count(&courses)
			database.DB.Model(&models.Lesson{}).Count(&lessons)
			database.DB.Model(&models.Certificate{}).Count(&certs)
			return blocksdk.BlockStats{
				BlockID: env.BlockID,
				Status:  "running",
				Metrics: map[string]any{"courses": courses, "lessons": lessons, "certificates": certs},
				Summary: fmt.Sprintf("%d คอร์ส • %d บทเรียน • %d เกียรติบัตร", courses, lessons, certs),
			}
		},
		RPCHandlers: map[string]blocksdk.RPCHandler{
			"courses": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				var courses []models.Course
				database.DB.Order("id desc").Limit(10).Find(&courses)
				return courses, nil
			},
		},
	})
}
