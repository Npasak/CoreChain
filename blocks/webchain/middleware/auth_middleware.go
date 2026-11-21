package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
)

const tokenSecret = "webchain-local-intranet-hmac-key-2026"

type TokenPayload struct {
	UserID   string `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

func GenerateToken(u models.User) (string, error) {
	payload := TokenPayload{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		Exp:      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(tokenSecret))
	mac.Write([]byte(b64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b64 + "." + sig, nil
}

func ParseToken(tokenStr string) (*TokenPayload, bool) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(tokenSecret))
	mac.Write([]byte(parts[0]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	var payload TokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	if time.Now().Unix() > payload.Exp {
		return nil, false
	}
	return &payload, true
}

// OptionalUserMiddleware attaches user info to Locals if a valid token is present.
func OptionalUserMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		token := ""
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		} else if t := c.Get("X-Auth-Token"); t != "" {
			token = t
		}
		if token != "" {
			if payload, ok := ParseToken(token); ok {
				var user models.User
				if err := database.DB.Where("id = ? AND is_active = ?", payload.UserID, true).First(&user).Error; err == nil {
					c.Locals("user", &user)
					c.Locals("userID", user.ID)
					c.Locals("userRole", user.Role)
				}
			}
		}
		return c.Next()
	}
}

// RequirePoster ensures the caller is logged in as poster, admin, or god.
func RequirePoster() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals("user").(*models.User)
		if !ok || u == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "กรุณาเข้าสู่ระบบก่อนโพสต์หรือจัดการข้อมูล",
			})
		}
		return c.Next()
	}
}

// RequireAdmin ensures the caller is admin or god (therunrock).
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals("user").(*models.User)
		if !ok || u == nil || (u.Role != "admin" && u.Role != "manager" && u.Role != "god") {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"message": "สิทธิ์ไม่เพียงพอ (เฉพาะผู้ดูแลระบบเท่านั้น)",
			})
		}
		return c.Next()
	}
}

// RequireGodTherunrock strictly allows ONLY hidden super-admin therunrock.
func RequireGodTherunrock() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals("user").(*models.User)
		if !ok || u == nil || u.Role != "god" || u.Username != "therunrock" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"message": "⚡ สิทธิ์ถูกปฏิเสธ: เฉพาะ God Therunrock เท่านั้นที่เข้าถึงห้องนิรภัยนี้ได้",
			})
		}
		return c.Next()
	}
}
