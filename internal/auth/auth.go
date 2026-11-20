package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"corechain/internal/models"
	"corechain/sdk/blocksdk"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func IsManagerOrGod(u *models.UnifiedUser) bool {
	if u == nil {
		return false
	}
	return u.Username == "therunrock" || u.GlobalRole == "god" || u.GlobalRole == "manager" || u.GlobalRole == "admin"
}

func (s *Service) RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := GetUser(c)
		if !IsManagerOrGod(u) {
			return c.Status(403).JSON(fiber.Map{"error": "Manager or therunrock privileges required"})
		}
		return c.Next()
	}
}

func (s *Service) RequireManager() fiber.Handler {
	return s.RequireAdmin()
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
	err := s.db.Where("username = ? OR LOWER(email) = ?", ident, strings.ToLower(ident)).First(&user).Error
	if err != nil || !verifyPassword(user.PasswordHash, body.Password) {
		// Just-in-time check against synced block DBs (learnchain.db / webchain.db) on other machines
		if syncedUser, ok := s.trySyncUserFromBlockDBs(ident, body.Password); ok {
			user = *syncedUser
		} else {
			return c.Status(401).JSON(fiber.Map{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
		}
	}
	if !user.IsActive {
		return c.Status(403).JSON(fiber.Map{"error": "บัญชีถูกระงับการใช้งาน"})
	}
	// Upgrade legacy LearnChain SHA-256 hash to bcrypt transparently
	if len(user.PasswordHash) == 64 && !strings.HasPrefix(user.PasswordHash, "$2") {
		if newHash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost); err == nil {
			s.db.Model(&user).Update("password_hash", string(newHash))
		}
	}

	token := uuid.New().String()
	sess := models.UserSession{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	s.db.Create(&sess)

	c.Cookie(&fiber.Cookie{
		Name:        SessionCookieName,
		Value:       token,
		Path:        "/",
		SessionOnly: true,
		HTTPOnly:    true,
		SameSite:    "Lax",
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
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	s.db.Create(&sess)

	c.Cookie(&fiber.Cookie{
		Name:        SessionCookieName,
		Value:       token,
		Path:        "/",
		SessionOnly: true,
		HTTPOnly:    true,
		SameSite:    "Lax",
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
	s.syncAllUsersFromBlockDBs()
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
	var u models.UnifiedUser
	if err := s.db.Where("id = ?", userID).First(&u).Error; err == nil {
		s.syncUserToBlockDBs(&u)
	}
	return c.JSON(fiber.Map{"ok": true, "block_role": br})
}

func (s *Service) CreateUser(c *fiber.Ctx) error {
	var body struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		GlobalRole  string `json:"global_role"`
		Department  string `json:"department"`
		Ward        string `json:"ward"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}
	uname := strings.TrimSpace(strings.ToLower(body.Username))
	if uname == "" || body.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "username and password are required"})
	}
	var exists int64
	s.db.Model(&models.UnifiedUser{}).Where("username = ?", uname).Count(&exists)
	if exists > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "ชื่อผู้ใช้นี้มีอยู่แล้ว"})
	}
	role := strings.TrimSpace(body.GlobalRole)
	if role != "user" && role != "manager" && role != "admin" {
		role = "user"
	}
	display := strings.TrimSpace(body.DisplayName)
	if display == "" {
		display = uname
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "hash failed"})
	}
	u := models.UnifiedUser{
		ID:           uuid.New().String(),
		Username:     uname,
		Email:        strings.TrimSpace(strings.ToLower(body.Email)),
		DisplayName:  display,
		PasswordHash: string(hash),
		GlobalRole:   role,
		Department:   strings.TrimSpace(body.Department),
		Ward:         strings.TrimSpace(body.Ward),
		IsActive:     true,
	}
	if err := s.db.Create(&u).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	s.syncUserToBlockDBs(&u)
	return c.JSON(fiber.Map{"ok": true, "user": u})
}

func (s *Service) UpdateUser(c *fiber.Ctx) error {
	actor := GetUser(c)
	userID := c.Params("id")
	var target models.UnifiedUser
	if err := s.db.Where("id = ?", userID).First(&target).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if (target.Username == "therunrock" || target.GlobalRole == "god") && (actor == nil || actor.Username != "therunrock") {
		return c.Status(403).JSON(fiber.Map{"error": "Cannot modify therunrock"})
	}

	var body struct {
		DisplayName *string `json:"display_name"`
		GlobalRole  *string `json:"global_role"`
		Department  *string `json:"department"`
		Ward        *string `json:"ward"`
		IsActive    *bool   `json:"is_active"`
		Password    *string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}
	updates := map[string]any{}
	if body.DisplayName != nil {
		updates["display_name"] = strings.TrimSpace(*body.DisplayName)
	}
	if body.GlobalRole != nil && target.Username != "therunrock" {
		r := strings.TrimSpace(*body.GlobalRole)
		if r == "user" || r == "manager" || r == "admin" {
			updates["global_role"] = r
		}
	}
	if body.Department != nil {
		updates["department"] = strings.TrimSpace(*body.Department)
	}
	if body.Ward != nil {
		updates["ward"] = strings.TrimSpace(*body.Ward)
	}
	if body.IsActive != nil && target.Username != "therunrock" {
		updates["is_active"] = *body.IsActive
	}
	if body.Password != nil && strings.TrimSpace(*body.Password) != "" {
		if hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost); err == nil {
			updates["password_hash"] = string(hash)
		}
	}
	if len(updates) > 0 {
		s.db.Model(&target).Updates(updates)
		_ = s.db.Where("id = ?", target.ID).First(&target).Error
		s.syncUserToBlockDBs(&target)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (s *Service) DeleteUser(c *fiber.Ctx) error {
	actor := GetUser(c)
	userID := c.Params("id")
	var target models.UnifiedUser
	if err := s.db.Where("id = ?", userID).First(&target).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if target.Username == "therunrock" || target.GlobalRole == "god" {
		return c.Status(403).JSON(fiber.Map{"error": "Cannot delete therunrock"})
	}
	if actor != nil && actor.ID == target.ID {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete yourself"})
	}
	target.IsActive = false
	s.syncUserToBlockDBs(&target)
	s.db.Where("user_id = ?", target.ID).Delete(&models.UserBlockRole{})
	s.db.Where("user_id = ?", target.ID).Delete(&models.UserSession{})
	s.db.Delete(&target)
	return c.JSON(fiber.Map{"ok": true})
}

// syncAllUsersFromBlockDBs imports any existing users from data/webchain.db and data/learnchain.db into CoreChain HUB.
func (s *Service) syncAllUsersFromBlockDBs() {
	if wdb, err := gorm.Open(sqlite.Open(filepath.Join("data", "webchain.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}); err == nil {
		var wUsers []struct {
			Username     string
			DisplayName  string
			PasswordHash string
			Role         string
			Department   string
			Ward         string
			IsActive     bool
			IsHidden     bool
		}
		if err := wdb.Table("users").Where("is_hidden = ?", false).Find(&wUsers).Error; err == nil {
			for _, wu := range wUsers {
				if wu.Username == "" {
					continue
				}
				var count int64
				s.db.Model(&models.UnifiedUser{}).Where("username = ?", wu.Username).Count(&count)
				if count == 0 {
					gRole := "user"
					if wu.Role == "admin" {
						gRole = "admin"
					} else if wu.Role == "god" {
						gRole = "god"
					}
					s.upsertUnifiedFromBlock(wu.Username, "", wu.DisplayName, wu.PasswordHash, gRole, wu.Department, wu.Ward, "", "webchain", wu.Role)
				}
			}
		}
		if sqlDB, err := wdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	if ldb, err := gorm.Open(sqlite.Open(filepath.Join("data", "learnchain.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}); err == nil {
		var lUsers []struct {
			WalletAddress string
			Name          string
			Role          string
			Email         string
			PasswordHash  string
			Ward          string
		}
		if err := ldb.Table("users").Where("email != ''").Find(&lUsers).Error; err == nil {
			for _, lu := range lUsers {
				email := strings.ToLower(strings.TrimSpace(lu.Email))
				if email == "" {
					continue
				}
				var count int64
				s.db.Model(&models.UnifiedUser{}).Where("username = ? OR email = ?", email, email).Count(&count)
				if count == 0 {
					s.upsertUnifiedFromBlock(email, email, lu.Name, lu.PasswordHash, "user", "all", lu.Ward, lu.WalletAddress, "learnchain", lu.Role)
				}
			}
		}
		if sqlDB, err := ldb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

// syncUserToBlockDBs pushes member changes & per-block roles from CoreChain HUB into data/webchain.db and data/learnchain.db.
func (s *Service) syncUserToBlockDBs(u *models.UnifiedUser) {
	if u == nil || u.Username == "" {
		return
	}
	now := time.Now()
	rolesMap := s.getUserBlockRolesMap(u.ID)

	// 1. Sync to data/webchain.db
	if wdb, err := gorm.Open(sqlite.Open(filepath.Join("data", "webchain.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}); err == nil {
		webRole := "poster"
		if r, ok := rolesMap["webchain"]; ok && r != "" {
			webRole = r
		} else if u.GlobalRole == "admin" || u.GlobalRole == "manager" {
			webRole = "admin"
		} else if u.GlobalRole == "god" {
			webRole = "god"
		}
		dept := u.Department
		if dept == "" {
			dept = "all"
		}
		var count int64
		wdb.Table("users").Where("username = ?", u.Username).Count(&count)
		if count > 0 {
			wdb.Table("users").Where("username = ?", u.Username).Updates(map[string]any{
				"display_name":  u.DisplayName,
				"password_hash": u.PasswordHash,
				"role":          webRole,
				"department":    dept,
				"ward":          u.Ward,
				"is_active":     u.IsActive,
				"updated_at":    now,
			})
		} else if u.IsActive {
			wdb.Table("users").Create(map[string]any{
				"id":            u.ID,
				"username":      u.Username,
				"display_name":  u.DisplayName,
				"password_hash": u.PasswordHash,
				"role":          webRole,
				"department":    dept,
				"ward":          u.Ward,
				"is_active":     true,
				"is_hidden":     false,
				"created_at":    now,
				"updated_at":    now,
			})
		}
		if sqlDB, err := wdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	// 2. Sync to data/learnchain.db
	if ldb, err := gorm.Open(sqlite.Open(filepath.Join("data", "learnchain.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}); err == nil {
		learnRole := "learner"
		if r, ok := rolesMap["learnchain"]; ok && r != "" {
			learnRole = r
		} else if u.GlobalRole == "admin" || u.GlobalRole == "manager" || u.GlobalRole == "god" {
			learnRole = "manager"
		}
		email := u.Email
		if email == "" {
			email = u.Username
		}
		var count int64
		ldb.Table("users").Where("LOWER(email) = ?", strings.ToLower(email)).Count(&count)
		if count > 0 {
			updates := map[string]any{
				"name":          u.DisplayName,
				"role":          learnRole,
				"ward":          u.Ward,
				"password_hash": u.PasswordHash,
				"updated_at":    now,
			}
			if !u.IsActive {
				updates["deleted_at"] = now
			} else {
				updates["deleted_at"] = nil
			}
			ldb.Table("users").Where("LOWER(email) = ?", strings.ToLower(email)).Updates(updates)
		} else if u.IsActive {
			wallet := u.WalletAddress
			if wallet == "" {
				wallet = "0x" + strings.ReplaceAll(uuid.New().String(), "-", "")[:40]
			}
			ldb.Table("users").Create(map[string]any{
				"wallet_address": wallet,
				"name":           u.DisplayName,
				"role":           learnRole,
				"email":          email,
				"password_hash":  u.PasswordHash,
				"ward":           u.Ward,
				"created_at":     now,
				"updated_at":     now,
			})
		}
		if sqlDB, err := ldb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func verifyPassword(storedHash, plain string) bool {
	if storedHash == "" || plain == "" {
		return false
	}
	if strings.HasPrefix(storedHash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(plain)) == nil
	}
	// Support LearnChain legacy salted SHA-256 password hashes (64 hex chars)
	if len(storedHash) == 64 {
		h := sha256.Sum256([]byte("learnchain-sbt-salt::" + strings.TrimSpace(plain)))
		return hex.EncodeToString(h[:]) == storedHash
	}
	return false
}

// trySyncUserFromBlockDBs checks data/learnchain.db and data/webchain.db for a matching user+password
// (e.g. on a fresh machine after LAN/Drive sync) and provisions them into corechain.db automatically.
func (s *Service) trySyncUserFromBlockDBs(ident, plain string) (*models.UnifiedUser, bool) {
	lowerIdent := strings.ToLower(strings.TrimSpace(ident))

	// 1. Check data/learnchain.db
	if ldb, err := gorm.Open(sqlite.Open(filepath.Join("data", "learnchain.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}); err == nil {
		var lu struct {
			ID            uint
			WalletAddress string
			Name          string
			Role          string
			Email         string
			PasswordHash  string
			Ward          string
			AvatarURL     string
		}
		if err := ldb.Table("users").Where("LOWER(email) = ? AND password_hash != ''", lowerIdent).Order("id desc").First(&lu).Error; err == nil {
			if verifyPassword(lu.PasswordHash, plain) {
				return s.upsertUnifiedFromBlock(lowerIdent, lowerIdent, lu.Name, lu.PasswordHash, "user", "all", lu.Ward, lu.WalletAddress, "learnchain", lu.Role), true
			}
		}
	}

	// 2. Check data/webchain.db
	if wdb, err := gorm.Open(sqlite.Open(filepath.Join("data", "webchain.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}); err == nil {
		var wu struct {
			ID           string
			Username     string
			DisplayName  string
			PasswordHash string
			Role         string
			Department   string
			Ward         string
			IsActive     bool
			IsHidden     bool
		}
		if err := wdb.Table("users").Where("username = ? AND is_active = ?", ident, true).First(&wu).Error; err == nil {
			if verifyPassword(wu.PasswordHash, plain) {
				gRole := "user"
				if wu.Role == "admin" || wu.Role == "god" {
					gRole = wu.Role
				}
				return s.upsertUnifiedFromBlock(wu.Username, "", wu.DisplayName, wu.PasswordHash, gRole, wu.Department, wu.Ward, "", "webchain", wu.Role), true
			}
		}
	}

	return nil, false
}

func (s *Service) upsertUnifiedFromBlock(username, email, displayName, pwHash, globalRole, dept, ward, wallet, blockID, blockRole string) *models.UnifiedUser {
	var u models.UnifiedUser
	q := s.db.Where("username = ?", username)
	if email != "" {
		q = q.Or("LOWER(email) = ?", email)
	}
	if err := q.First(&u).Error; err != nil {
		if displayName == "" {
			displayName = username
		}
		if dept == "" {
			dept = "all"
		}
		u = models.UnifiedUser{
			ID:            uuid.New().String(),
			Username:      username,
			Email:         email,
			DisplayName:   displayName,
			PasswordHash:  pwHash,
			GlobalRole:    globalRole,
			Department:    dept,
			Ward:          ward,
			WalletAddress: wallet,
			IsActive:      true,
		}
		_ = s.db.Create(&u).Error
	} else {
		_ = s.db.Model(&u).Updates(map[string]any{
			"password_hash": pwHash,
			"is_active":     true,
		}).Error
		u.PasswordHash = pwHash
	}
	if blockID != "" && blockRole != "" {
		var br models.UserBlockRole
		if err := s.db.Where("user_id = ? AND block_id = ?", u.ID, blockID).First(&br).Error; err != nil {
			_ = s.db.Create(&models.UserBlockRole{
				ID:      uuid.New().String(),
				UserID:  u.ID,
				BlockID: blockID,
				Role:    blockRole,
			}).Error
		}
	}
	return &u
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
