package handlers

import (
	"strings"
	"time"

	"webchain/database"
	"webchain/middleware"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const SecretAdminUsername = "therunrock"

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}
	username := strings.TrimSpace(body.Username)
	if username == "" || body.Password == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณากรอกชื่อผู้ใช้และรหัสผ่าน"})
	}

	var user models.User
	if err := database.DB.Where("LOWER(username) = ?", strings.ToLower(username)).First(&user).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
	}
	if !user.IsActive {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "บัญชีนี้ถูกระงับการใช้งาน"})
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
	}

	token, err := middleware.GenerateToken(user)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "สร้าง Token ไม่สำเร็จ"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"token":   token,
		"user":    user,
	})
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Department  string `json:"department"`
		Ward        string `json:"ward"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}
	username := strings.TrimSpace(body.Username)
	displayName := strings.TrimSpace(body.DisplayName)
	if username == "" || displayName == "" || len(body.Password) < 4 {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณากรอกชื่อผู้ใช้ ชื่อแสดงผล และรหัสผ่านอย่างน้อย 4 ตัวอักษร"})
	}
	if strings.EqualFold(username, SecretAdminUsername) {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "ไม่สามารถใช้ชื่อผู้ใช้นี้ได้"})
	}

	var existing int64
	database.DB.Unscoped().Model(&models.User{}).Where("LOWER(username) = ?", strings.ToLower(username)).Count(&existing)
	if existing > 0 {
		return c.Status(409).JSON(fiber.Map{"success": false, "message": "ชื่อผู้ใช้นี้มีอยู่ในระบบแล้ว"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "ไม่สามารถเข้ารหัสรหัสผ่านได้"})
	}

	dept := body.Department
	if dept == "" {
		dept = models.DeptAll
	}
	user := models.User{
		ID:           uuid.NewString(),
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: string(hash),
		Role:         "poster",
		Department:   dept,
		Ward:         strings.TrimSpace(body.Ward),
		IsActive:     true,
		IsHidden:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := database.DB.Create(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "บันทึกผู้ใช้ไม่สำเร็จ"})
	}

	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: user.DisplayName,
		Action:    "register",
		Target:    user.Username,
		Detail:    "ลงทะเบียนผู้โพสต์ใหม่ (" + user.Ward + ")",
		CreatedAt: time.Now(),
	})

	token, _ := middleware.GenerateToken(user)
	return c.JSON(fiber.Map{
		"success": true,
		"token":   token,
		"user":    user,
	})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	u, ok := c.Locals("user").(*models.User)
	if !ok || u == nil {
		return c.JSON(fiber.Map{"success": true, "user": nil})
	}
	return c.JSON(fiber.Map{"success": true, "user": u})
}

// ListUsers returns all visible users (never leaks hidden therunrock account)
func (h *AuthHandler) ListUsers(c *fiber.Ctx) error {
	var users []models.User
	database.DB.Where("is_hidden = ?", false).Order("created_at desc").Find(&users)
	return c.JSON(fiber.Map{"success": true, "users": users})
}

func (h *AuthHandler) UpdateUserRole(c *fiber.Ctx) error {
	id := c.Params("id")
	var body struct {
		Role     string `json:"role"`
		IsActive *bool  `json:"is_active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}

	var target models.User
	if err := database.DB.Where("id = ? AND is_hidden = ?", id, false).First(&target).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบผู้ใช้"})
	}

	if body.Role == "poster" || body.Role == "admin" {
		target.Role = body.Role
	}
	if body.IsActive != nil {
		target.IsActive = *body.IsActive
	}
	target.UpdatedAt = time.Now()
	database.DB.Save(&target)

	return c.JSON(fiber.Map{"success": true, "user": target})
}
