package auth

import (
	"net/http"
	"strings"
	"time"

	"corechain/internal/models"
	"corechain/sdk/blocksdk"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const SessionCookieName = "corechain_session"

type Service struct {
	db     *gorm.DB
	secret string
}

func NewService(db *gorm.DB, secret string) *Service {
	return &Service{db: db, secret: secret}
}

// OptionalUserMiddleware resolves the current UnifiedUser from Cookie or Authorization/X-Auth-Token header.
func (s *Service) OptionalUserMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := s.extractToken(c)
		if token != "" {
			var sess models.UserSession
			if err := s.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&sess).Error; err == nil {
				var user models.UnifiedUser
				if err := s.db.Where("id = ? AND is_active = ?", sess.UserID, true).First(&user).Error; err == nil {
					c.Locals("unified_user", &user)
				}
			}
		}
		return c.Next()
	}
}

func (s *Service) RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := GetUser(c)
		if u == nil || (u.GlobalRole != "admin" && u.GlobalRole != "god") {
			return c.Status(403).JSON(fiber.Map{"error": "Admin privileges required"})
		}
		return c.Next()
	}
}

func (s *Service) RequireGod() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := GetUser(c)
		if u == nil || (u.Username != "therunrock" && u.GlobalRole != "god") {
			return c.Status(403).JSON(fiber.Map{"error": "God Vault access denied"})
		}
		return c.Next()
	}
}

func GetUser(c *fiber.Ctx) *models.UnifiedUser {
	if v := c.Locals("unified_user"); v != nil {
		if u, ok := v.(*models.UnifiedUser); ok {
			return u
		}
	}
	return nil
}

func (s *Service) extractToken(c *fiber.Ctx) string {
	if tok := c.Cookies(SessionCookieName); tok != "" {
		return tok
	}
	if tok := c.Get("X-Auth-Token"); tok != "" {
		return tok
	}
	authz := c.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") {
		return strings.TrimPrefix(authz, "Bearer ")
	}
	return ""
}

// InjectGatewayHeaders signs and attaches Unified Auth headers for a proxied request to a block.
func (s *Service) InjectGatewayHeaders(c *fiber.Ctx, blockID string, req *http.Request) {
	u := GetUser(c)
	if u == nil {
		return
	}
	blockRole := s.GetBlockRole(u, blockID)
	cu := blocksdk.CoreUser{
		UserID:      u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		GlobalRole:  u.GlobalRole,
		BlockRole:   blockRole,
		Ward:        u.Ward,
		Department:  u.Department,
		WalletAddr:  u.WalletAddress,
	}
	sig := blocksdk.SignCoreUser(s.secret, cu)

	req.Header.Set("X-Core-User-ID", cu.UserID)
	req.Header.Set("X-Core-Username", cu.Username)
	req.Header.Set("X-Core-Display-Name", cu.DisplayName)
	req.Header.Set("X-Core-Email", cu.Email)
	req.Header.Set("X-Core-Global-Role", cu.GlobalRole)
	req.Header.Set("X-Core-Block-Role", cu.BlockRole)
	req.Header.Set("X-Core-Ward", cu.Ward)
	req.Header.Set("X-Core-Dept", cu.Department)
	req.Header.Set("X-Core-Wallet", cu.WalletAddr)
	req.Header.Set("X-Core-Sig", sig)
}

func (s *Service) GetBlockRole(u *models.UnifiedUser, blockID string) string {
	if u.GlobalRole == "god" || u.GlobalRole == "admin" {
		return "admin"
	}
	var br models.UserBlockRole
	if err := s.db.Where("user_id = ? AND block_id = ?", u.ID, blockID).First(&br).Error; err == nil {
		return br.Role
	}
	return "user"
}

func (s *Service) Login(c *fiber.Ctx) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	ident := strings.TrimSpace(body.Username)
	if ident == "" || body.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "กรุณากรอกชื่อผู้ใช้และรหัสผ่าน"})
	}

	var user models.UnifiedUser
	if err := s.db.Where("username = ? OR LOWER(email) = ?", ident, strings.ToLower(ident)).First(&user).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
	}
	if !user.IsActive {
		return c.Status(403).JSON(fiber.Map{"error": "บัญชีถูกระงับการใช้งาน"})
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
	}

	token := uuid.New().String()
	sess := models.UserSession{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	s.db.Create(&sess)

	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HTTPOnly: true,
		SameSite: "Lax",
	})

	roles := s.getUserBlockRolesMap(user.ID)
	return c.JSON(fiber.Map{
		"token":       token,
		"user":        user,
		"block_roles": roles,
	})
}

func (s *Service) Register(c *fiber.Ctx) error {
	var body struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Ward        string `json:"ward"`
		Department  string `json:"department"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	uname := strings.TrimSpace(body.Username)
	if uname == "" || len(body.Password) < 4 {
		return c.Status(400).JSON(fiber.Map{"error": "Username และรหัสผ่าน (อย่างน้อย 4 ตัวอักษร) จำเป็นต้องกรอก"})
	}
	var count int64
	s.db.Model(&models.UnifiedUser{}).Where("username = ?", uname).Count(&count)
	if count > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "ชื่อผู้ใช้นี้มีอยู่แล้ว"})
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	display := strings.TrimSpace(body.DisplayName)
	if display == "" {
		display = uname
	}
	dept := body.Department
	if dept == "" {
		dept = "all"
	}
	user := models.UnifiedUser{
		ID:           uuid.New().String(),
		Username:     uname,
		Email:        strings.TrimSpace(strings.ToLower(body.Email)),
		DisplayName:  display,
		PasswordHash: string(hash),
		GlobalRole:   "user",
		Department:   dept,
		Ward:         body.Ward,
		IsActive:     true,
	}
	if err := s.db.Create(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	token := uuid.New().String()
	sess := models.UserSession{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	s.db.Create(&sess)

	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HTTPOnly: true,
		SameSite: "Lax",
	})
	return c.Status(201).JSON(fiber.Map{
		"token":       token,
		"user":        user,
		"block_roles": map[string]string{},
	})
}

func (s *Service) Logout(c *fiber.Ctx) error {
	tok := s.extractToken(c)
	if tok != "" {
		s.db.Where("token = ?", tok).Delete(&models.UserSession{})
	}
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
	})
	return c.JSON(fiber.Map{"ok": true})
}

func (s *Service) Me(c *fiber.Ctx) error {
	u := GetUser(c)
	if u == nil {
		return c.JSON(fiber.Map{"authenticated": false, "user": nil})
	}
	return c.JSON(fiber.Map{
		"authenticated": true,
		"user":          u,
		"block_roles":   s.getUserBlockRolesMap(u.ID),
	})
}

func (s *Service) ListUsers(c *fiber.Ctx) error {
	var users []models.UnifiedUser
	s.db.Where("is_hidden = ?", false).Order("created_at desc").Find(&users)
	type userWithRoles struct {
		models.UnifiedUser
		BlockRoles map[string]string `json:"block_roles"`
	}
	out := make([]userWithRoles, 0, len(users))
	for _, u := range users {
		out = append(out, userWithRoles{
			UnifiedUser: u,
			BlockRoles:  s.getUserBlockRolesMap(u.ID),
		})
	}
	return c.JSON(fiber.Map{"users": out})
}

func (s *Service) SetBlockRole(c *fiber.Ctx) error {
	userID := c.Params("id")
	var body struct {
		BlockID string `json:"block_id"`
		Role    string `json:"role"`
	}
	if err := c.BodyParser(&body); err != nil || body.BlockID == "" || body.Role == "" {
		return c.Status(400).JSON(fiber.Map{"error": "block_id and role are required"})
	}
	var br models.UserBlockRole
	err := s.db.Where("user_id = ? AND block_id = ?", userID, body.BlockID).First(&br).Error
	if err != nil {
		br = models.UserBlockRole{
			ID:      uuid.New().String(),
			UserID:  userID,
			BlockID: body.BlockID,
			Role:    body.Role,
		}
		s.db.Create(&br)
	} else {
		s.db.Model(&br).Update("role", body.Role)
	}
	return c.JSON(fiber.Map{"ok": true, "block_role": br})
}

func (s *Service) getUserBlockRolesMap(userID string) map[string]string {
	var roles []models.UserBlockRole
	s.db.Where("user_id = ?", userID).Find(&roles)
	m := make(map[string]string, len(roles))
	for _, r := range roles {
		m[r.BlockID] = r.Role
	}
	return m
}
