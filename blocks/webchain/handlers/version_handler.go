package handlers

import (
	"webchain/version"

	"github.com/gofiber/fiber/v2"
)

type VersionHandler struct{}

func NewVersionHandler() *VersionHandler {
	return &VersionHandler{}
}

func (h *VersionHandler) GetVersion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success":    true,
		"app":        "WebChain",
		"version":    version.Version,
		"build_date": version.BuildDate,
	})
}
