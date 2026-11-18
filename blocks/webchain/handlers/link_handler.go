package handlers

import (
	"net/http"
	"strings"
	"time"

	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type LinkHandler struct{}

func NewLinkHandler() *LinkHandler {
	return &LinkHandler{}
}

func (h *LinkHandler) ListLinks(c *fiber.Ctx) error {
	var links []models.ExternalLink
	database.DB.Order("sort_order asc, key asc").Find(&links)
	return c.JSON(fiber.Map{"success": true, "links": links})
}

func (h *LinkHandler) UpsertLink(c *fiber.Ctx) error {
	var body struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		URL         string `json:"url"`
		Status      string `json:"status"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}
	key := strings.TrimSpace(body.Key)
	if key == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณาระบุ key"})
	}

	var link models.ExternalLink
	err := database.DB.Where("key = ?", key).First(&link).Error
	if err != nil {
		link = models.ExternalLink{
			ID:        uuid.NewString(),
			Key:       key,
			SortOrder: 10,
		}
	}
	if body.Label != "" {
		link.Label = body.Label
	}
	link.URL = strings.TrimSpace(body.URL)
	if body.Status == "active" || body.Status == "coming_soon" {
		link.Status = body.Status
	}
	if body.Description != "" {
		link.Description = body.Description
	}
	if body.Icon != "" {
		link.Icon = body.Icon
	}
	link.UpdatedAt = time.Now()
	database.DB.Save(&link)

	return c.JSON(fiber.Map{"success": true, "link": link})
}

// CheckLearnChainPing checks if the configured LearnChain URL is reachable
func (h *LinkHandler) CheckLearnChainPing(c *fiber.Ctx) error {
	var link models.ExternalLink
	if err := database.DB.Where("key = ?", "learnchain").First(&link).Error; err != nil || link.URL == "" {
		return c.JSON(fiber.Map{"success": true, "online": false, "url": ""})
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get(strings.TrimRight(link.URL, "/") + "/api/version")
	online := err == nil && resp != nil && resp.StatusCode < 500
	if resp != nil {
		_ = resp.Body.Close()
	}
	return c.JSON(fiber.Map{
		"success": true,
		"online":  online,
		"url":     link.URL,
	})
}
