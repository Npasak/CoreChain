package models

import (
	"time"

	"gorm.io/gorm"
)

// UnifiedUser is the single sign-on identity across all CoreChain blocks.
type UnifiedUser struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	Username       string         `gorm:"uniqueIndex;not null" json:"username"`
	Email          string         `gorm:"index" json:"email"`
	DisplayName    string         `gorm:"not null" json:"display_name"`
	PasswordHash   string         `gorm:"not null" json:"-"`
	GlobalRole     string         `gorm:"default:'user'" json:"global_role"` // user, admin, god
	Department     string         `gorm:"default:'all'" json:"department"`
	Ward           string         `gorm:"default:''" json:"ward"`
	WalletAddress  string         `gorm:"index" json:"wallet_address,omitempty"`
	AvatarURL      string         `json:"avatar_url,omitempty"`
	IsActive       bool           `gorm:"default:true" json:"is_active"`
	IsHidden       bool           `gorm:"default:false" json:"is_hidden"`
	LegacyWebID    string         `json:"legacy_web_id,omitempty"`
	LegacyLearnID  uint           `json:"legacy_learn_id,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// UserBlockRole stores per-block role overrides for a UnifiedUser.
type UserBlockRole struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	UserID    string    `gorm:"index:idx_user_block,unique;not null" json:"user_id"`
	BlockID   string    `gorm:"index:idx_user_block,unique;not null" json:"block_id"`
	Role      string    `gorm:"not null" json:"role"` // e.g. poster/admin (webchain), learner/creator/manager (learnchain), editor/viewer (docchain)
	UpdatedAt time.Time `json:"updated_at"`
}

// UserSession stores active login sessions for Unified Auth.
type UserSession struct {
	Token     string    `gorm:"primaryKey;type:text" json:"token"`
	UserID    string    `gorm:"index;not null" json:"user_id"`
	ExpiresAt time.Time `gorm:"index;not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// InstalledBlock tracks installed .Blz blocks, their runtime config, and SHA256 checksum.
type InstalledBlock struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CPUThreads  int       `gorm:"default:0" json:"cpu_threads"`
	SortOrder   int       `gorm:"default:0" json:"sort_order"`
	PackageHash string    `json:"package_hash"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SystemSetting stores CoreChain node identity & decentralized sync configuration.
type SystemSetting struct {
	Key       string    `gorm:"primaryKey;type:text" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConnectorEventLog records recent Pub/Sub events on the Connector Bus for observability.
type ConnectorEventLog struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	Topic       string    `gorm:"index;not null" json:"topic"`
	SenderBlock string    `gorm:"index;not null" json:"sender_block"`
	PayloadJSON string    `gorm:"type:text" json:"payload_json"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// AuditLog records important CoreChain operations (install/update .Blz, backup, login).
type AuditLog struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	ActorID   string    `gorm:"index" json:"actor_id"`
	Action    string    `gorm:"index;not null" json:"action"`
	Target    string    `json:"target"`
	Detail    string    `gorm:"type:text" json:"detail"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
