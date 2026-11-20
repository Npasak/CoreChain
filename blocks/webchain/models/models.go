package models

import (
	"time"

	"gorm.io/gorm"
)

// Department & Ward constants
const (
	DeptAll      = "all"
	DeptSpecial2 = "special-2" // งานการพยาบาลผู้ป่วยพิเศษ 2
	DeptSpecial3 = "special-3" // งานการพยาบาลผู้ป่วยพิเศษ 3
)

var Special2Wards = []string{"หอสงฆ์", "สว11", "สว12"}
var Special3Wards = []string{"8B", "8C", "9A", "9B", "9C"}

// User represents a registered poster/admin (viewers do not need to log in)
type User struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	Username     string         `gorm:"uniqueIndex;not null" json:"username"`
	DisplayName  string         `gorm:"not null" json:"display_name"`
	PasswordHash string         `gorm:"not null" json:"-"`
	Role         string         `gorm:"default:'poster'" json:"role"`         // poster, admin, god
	Department   string         `gorm:"default:'all'" json:"department"`    // all, special-2, special-3
	Ward         string         `gorm:"default:''" json:"ward"`             // e.g. หอสงฆ์, สว11, 8A
	IsActive     bool           `gorm:"default:true" json:"is_active"`
	IsHidden     bool           `gorm:"default:false" json:"is_hidden"`     // true for therunrock
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// Post represents an award congratulation, meeting/training, or announcement
type Post struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	Category       string         `gorm:"index;not null" json:"category"` // award, meeting, announcement
	ScopeDept      string         `gorm:"index;default:'all'" json:"scope_dept"`
	TargetWards    string         `gorm:"type:text;default:'[\"all\"]'" json:"target_wards"` // JSON array string
	Title          string         `gorm:"not null" json:"title"`
	Content        string         `gorm:"type:text" json:"content"`
	ImageData      string         `gorm:"type:text" json:"image_data,omitempty"` // compressed base64 data URI (cover)
	Images         string         `gorm:"type:text;default:'[]'" json:"images,omitempty"` // JSON array of up to 5 compressed images
	AttachmentName string         `json:"attachment_name,omitempty"`
	IsPinned       bool           `gorm:"default:false" json:"is_pinned"`
	Priority       string         `gorm:"default:'normal'" json:"priority"` // normal, urgent

	// Award-specific fields
	HonoreeName string `json:"honoree_name,omitempty"`
	HonoreeWard string `json:"honoree_ward,omitempty"`
	AwardTitle  string `json:"award_title,omitempty"`

	// Meeting/Training-specific fields
	EventStartAt   *time.Time `json:"event_start_at,omitempty"`
	EventEndAt     *time.Time `json:"event_end_at,omitempty"`
	LocationOrLink string     `json:"location_or_link,omitempty"`
	Speaker        string     `json:"speaker,omitempty"`

	// Engagement & Attribution
	ApplauseCount int    `gorm:"default:0" json:"applause_count"`
	AuthorID      string `gorm:"index" json:"author_id"`
	AuthorName    string `json:"author_name"`
	AuthorWard    string `json:"author_ward"`

	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// ExternalLink stores links to LearnChain, DocSearch, and future hospital systems
type ExternalLink struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	Key         string    `gorm:"uniqueIndex;not null" json:"key"` // learnchain, docsearch
	Label       string    `gorm:"not null" json:"label"`
	URL         string    `json:"url"`
	Status      string    `gorm:"default:'active'" json:"status"` // active, coming_soon
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	SortOrder   int       `gorm:"default:0" json:"sort_order"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SystemSetting stores key-value settings synced across peers
type SystemSetting struct {
	Key       string    `gorm:"primaryKey;type:text" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditLog records important actions (post creation, deletion, backups, resets)
type AuditLog struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	ActorName string    `json:"actor_name"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// WardProfile stores detailed information for each sub-ward
type WardProfile struct {
	WardCode      string    `gorm:"primaryKey;type:text" json:"ward_code"`
	Department    string    `gorm:"index;not null" json:"department"` // special-2 | special-3
	DisplayName   string    `json:"display_name"`
	CoverImage    string    `gorm:"type:text" json:"cover_image,omitempty"`
	LeaderName    string    `json:"leader_name,omitempty"`
	LeaderTitle   string    `json:"leader_title,omitempty"`
	LeaderPhoto   string    `gorm:"type:text" json:"leader_photo,omitempty"`
	History       string    `gorm:"type:text" json:"history"`
	BedCount      int       `gorm:"default:0" json:"bed_count"`
	PatientGroups string    `gorm:"type:text" json:"patient_groups"`
	StaffLevels   string    `gorm:"type:text;default:'[]'" json:"staff_levels"` // JSON: [{level, count}]
	Roster        string    `gorm:"type:text;default:'[]'" json:"roster"`       // JSON: [{name, position, level, photo_data, birth_month, birth_day}]
	BasicStats    string    `gorm:"type:text;default:'[]'" json:"basic_stats"`  // JSON: [{label, value, unit}]
	UpdatedAt     time.Time `json:"updated_at"`
}

// BirthdayWish tracks monthly birthday wishes per member
type BirthdayWish struct {
	ID         string    `gorm:"primaryKey;type:text" json:"id"` // dept|ward|name|YYYY-MM
	Department string    `gorm:"index" json:"department"`
	WardCode   string    `json:"ward_code"`
	MemberName string    `json:"member_name"`
	YearMonth  string    `gorm:"index" json:"year_month"`
	WishCount  int       `gorm:"default:0" json:"wish_count"`
	UpdatedAt  time.Time `json:"updated_at"`
}
