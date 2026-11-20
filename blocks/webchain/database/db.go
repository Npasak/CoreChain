package database

import (
	"fmt"
	"log"
	"time"

	"webchain/config"
	"webchain/models"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(dbPath string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to SQLite database (%s): %v", dbPath, err)
	}

	// Enable WAL mode for better concurrency on Windows
	sqlDB, err := db.DB()
	if err == nil {
		_, _ = sqlDB.Exec("PRAGMA journal_mode=WAL;")
		_, _ = sqlDB.Exec("PRAGMA busy_timeout=5000;")
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Post{},
		&models.ExternalLink{},
		&models.SystemSetting{},
		&models.AuditLog{},
		&models.WardProfile{},
		&models.BirthdayWish{},
	)
	if err != nil {
		log.Fatalf("Failed to AutoMigrate WebChain tables: %v", err)
	}

	DB = db
	SeedInitialData(db)
	SeedWardProfiles(db)
	return db
}

func SeedWardProfiles(db *gorm.DB) {
	// Migrate legacy 8A ward to 8C if present
	db.Model(&models.WardProfile{}).Where("ward_code = ?", "8A").Updates(map[string]any{
		"ward_code":    "8C",
		"display_name": "หอผู้ป่วยพิเศษ 8C",
		"leader_title": "หัวหน้าหอผู้ป่วย 8C",
		"history":      "หอผู้ป่วยพิเศษ 8C งานการพยาบาลผู้ป่วยพิเศษ 3 ให้บริการดูแลผู้ป่วยอย่างอบอุ่นและปลอดภัย",
	})
	var count int64
	db.Model(&models.WardProfile{}).Count(&count)
	if count > 0 {
		return
	}
	defaultLevels := `[{"level":"หัวหน้าหอผู้ป่วย","count":1},{"level":"พยาบาลวิชาชีพ (RN)","count":12},{"level":"ผู้ช่วยพยาบาล (PN/NA)","count":5},{"level":"พนักงานบริการ","count":2}]`
	defaultStats := `[{"label":"อัตราครองเตียง","value":"85","unit":"%"},{"label":"วันนอนเฉลี่ย","value":"4.2","unit":"วัน"},{"label":"ความพึงพอใจ","value":"96","unit":"%"}]`
	now := time.Now()
	curMonth := int(now.Month())
	curDay := now.Day()
	sampleRosterS2 := fmt.Sprintf(`[{"name":"พว.สมใจ ใจดี","position":"หัวหน้าหอผู้ป่วย","birth_month":%d,"birth_day":%d},{"name":"พว.วิภา รักษา","position":"พยาบาลวิชาชีพ","birth_month":%d,"birth_day":22}]`, curMonth, curDay, curMonth)
	sampleRosterS3 := fmt.Sprintf(`[{"name":"พว.สุภาพร ดีงาม","position":"หัวหน้าหอผู้ป่วย","birth_month":%d,"birth_day":%d},{"name":"พว.ธนพล ดูแล","position":"พยาบาลวิชาชีพ","birth_month":%d,"birth_day":25}]`, curMonth, curDay, curMonth)

	wards := []models.WardProfile{
		{WardCode: "dept-special-2", Department: models.DeptSpecial2, DisplayName: "งานการพยาบาลผู้ป่วยพิเศษ 2", LeaderName: "พว.หัวหน้างาน พิเศษ 2", LeaderTitle: "หัวหน้างานการพยาบาลผู้ป่วยพิเศษ 2", History: "งานการพยาบาลผู้ป่วยพิเศษ 2 ดูแลหอผู้ป่วยสงฆ์, สว11 และ สว12 มุ่งเน้นบริการพยาบาลที่เป็นเลิศและอบอุ่น", BedCount: 56, PatientGroups: "พระภิกษุสงฆ์อาพาธ และผู้ป่วยพิเศษอายุรกรรม-ศัลยกรรม", StaffLevels: `[{"level":"หัวหน้างาน/หัวหน้าหอ","count":4},{"level":"พยาบาลวิชาชีพ (RN)","count":36},{"level":"ผู้ช่วยพยาบาล (PN/NA)","count":15},{"level":"พนักงานบริการ","count":6}]`, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "dept-special-3", Department: models.DeptSpecial3, DisplayName: "งานการพยาบาลผู้ป่วยพิเศษ 3", LeaderName: "พว.หัวหน้างาน พิเศษ 3", LeaderTitle: "หัวหน้างานการพยาบาลผู้ป่วยพิเศษ 3", History: "งานการพยาบาลผู้ป่วยพิเศษ 3 ดูแลหอผู้ป่วย 8B, 8C, 9A, 9B และ 9C ด้วยมาตรฐานวิชาชีพและความปลอดภัยสูงสุด", BedCount: 90, PatientGroups: "ผู้ป่วยพิเศษอายุรกรรม ศัลยกรรม และเฉพาะทาง", StaffLevels: `[{"level":"หัวหน้างาน/หัวหน้าหอ","count":6},{"level":"พยาบาลวิชาชีพ (RN)","count":60},{"level":"ผู้ช่วยพยาบาล (PN/NA)","count":25},{"level":"พนักงานบริการ","count":10}]`, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "หอสงฆ์", Department: models.DeptSpecial2, DisplayName: "หอผู้ป่วยสงฆ์", LeaderName: "พว.สมใจ ใจดี", LeaderTitle: "หัวหน้าหอผู้ป่วยสงฆ์", History: "หอผู้ป่วยที่จัดตั้งขึ้นเพื่อดูแลรักษาพระภิกษุสงฆ์และสามเณรอาพาธอย่างเหมาะสมตามพระธรรมวินัย", BedCount: 16, PatientGroups: "พระภิกษุสงฆ์และสามเณรอาพาธ (อายุรกรรม / ศัลยกรรม)", StaffLevels: defaultLevels, Roster: sampleRosterS2, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "สว11", Department: models.DeptSpecial2, DisplayName: "หอผู้ป่วยพิเศษ สว11", LeaderName: "พว.หัวหน้าหอ สว11", LeaderTitle: "หัวหน้าหอผู้ป่วย สว11", History: "หอผู้ป่วยพิเศษชั้น 11 ให้บริการดูแลผู้ป่วยในแบบห้องพิเศษเดี่ยวด้วยมาตรฐานวิชาชีพ", BedCount: 20, PatientGroups: "ผู้ป่วยพิเศษอายุรกรรม และศัลยกรรมทั่วไป", StaffLevels: defaultLevels, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "สว12", Department: models.DeptSpecial2, DisplayName: "หอผู้ป่วยพิเศษ สว12", LeaderName: "พว.หัวหน้าหอ สว12", LeaderTitle: "หัวหน้าหอผู้ป่วย สว12", History: "หอผู้ป่วยพิเศษชั้น 12 มุ่งเน้นการพยาบาลแบบองค์รวมและประสานทีมสหสาขาวิชาชีพ", BedCount: 20, PatientGroups: "ผู้ป่วยพิเศษอายุรกรรม ศัลยกรรม และเฉพาะทาง", StaffLevels: defaultLevels, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "8B", Department: models.DeptSpecial3, DisplayName: "หอผู้ป่วยพิเศษ 8B", LeaderName: "พว.หัวหน้าหอ 8B", LeaderTitle: "หัวหน้าหอผู้ป่วย 8B", History: "หอผู้ป่วยพิเศษ 8B เน้นการฟื้นฟูและดูแลหลังผ่าตัดอย่างใกล้ชิด", BedCount: 18, PatientGroups: "ผู้ป่วยพิเศษศัลยกรรมและออร์โธปิดิกส์", StaffLevels: defaultLevels, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "8C", Department: models.DeptSpecial3, DisplayName: "หอผู้ป่วยพิเศษ 8C", LeaderName: "พว.สุภาพร ดีงาม", LeaderTitle: "หัวหน้าหอผู้ป่วย 8C", History: "หอผู้ป่วยพิเศษ 8C งานการพยาบาลผู้ป่วยพิเศษ 3 ให้บริการดูแลผู้ป่วยอย่างอบอุ่นและปลอดภัย", BedCount: 18, PatientGroups: "ผู้ป่วยพิเศษอายุรกรรมและโรคเรื้อรัง", StaffLevels: defaultLevels, Roster: sampleRosterS3, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "9A", Department: models.DeptSpecial3, DisplayName: "หอผู้ป่วยพิเศษ 9A", LeaderName: "พว.หัวหน้าหอ 9A", LeaderTitle: "หัวหน้าหอผู้ป่วย 9A", History: "หอผู้ป่วยพิเศษ 9A ให้บริการห้องพิเศษมาตรฐานและการดูแลเฉพาะทาง", BedCount: 18, PatientGroups: "ผู้ป่วยพิเศษอายุรกรรมและเฉพาะทาง", StaffLevels: defaultLevels, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "9B", Department: models.DeptSpecial3, DisplayName: "หอผู้ป่วยพิเศษ 9B", LeaderName: "พว.หัวหน้าหอ 9B", LeaderTitle: "หัวหน้าหอผู้ป่วย 9B", History: "หอผู้ป่วยพิเศษ 9B มุ่งเน้นคุณภาพบริการและความปลอดภัยของผู้ป่วย", BedCount: 18, PatientGroups: "ผู้ป่วยพิเศษศัลยกรรมและนรีเวช", StaffLevels: defaultLevels, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
		{WardCode: "9C", Department: models.DeptSpecial3, DisplayName: "หอผู้ป่วยพิเศษ 9C", LeaderName: "พว.หัวหน้าหอ 9C", LeaderTitle: "หัวหน้าหอผู้ป่วย 9C", History: "หอผู้ป่วยพิเศษ 9C ดูแลผู้ป่วยพิเศษด้วยทีมพยาบาลผู้เชี่ยวชาญตลอด 24 ชั่วโมง", BedCount: 18, PatientGroups: "ผู้ป่วยพิเศษสหสาขา", StaffLevels: defaultLevels, Roster: `[]`, BasicStats: defaultStats, UpdatedAt: now},
	}
	for _, w := range wards {
		db.Create(&w)
	}
}

func SeedInitialData(db *gorm.DB) {
	// 1. Ensure hidden therunrock super-admin exists
	var godCount int64
	db.Unscoped().Model(&models.User{}).Where("username = ?", "therunrock").Count(&godCount)
	if godCount == 0 {
		secret := "therunrock2026"
		if config.AppConfig != nil && config.AppConfig.TherunrockSecret != "" {
			secret = config.AppConfig.TherunrockSecret
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		db.Create(&models.User{
			ID:           "god-therunrock-001",
			Username:     "therunrock",
			DisplayName:  "Godess (therunrock)",
			PasswordHash: string(hash),
			Role:         "god",
			Department:   models.DeptAll,
			Ward:         "System",
			IsActive:     true,
			IsHidden:     true,
		})
	}

	// 2. Seed default external links if empty
	var linkCount int64
	db.Model(&models.ExternalLink{}).Count(&linkCount)
	if linkCount == 0 {
		lcURL := "http://localhost:8888"
		dsURL := ""
		dsStatus := "coming_soon"
		if config.AppConfig != nil {
			lcURL = config.AppConfig.LearnChainURL
			dsURL = config.AppConfig.DocSearchURL
			dsStatus = config.AppConfig.DocSearchStatus
		}
		db.Create(&models.ExternalLink{
			ID:          uuid.NewString(),
			Key:         "learnchain",
			Label:       "ระบบ E-Learning (LearnChain)",
			URL:         lcURL,
			Status:      "active",
			Description: "เข้าสู่ระบบอบรมออนไลน์และใบรับรองร่วมทั้ง 2 หน่วยงาน",
			Icon:        "🎓",
			SortOrder:   1,
		})
		db.Create(&models.ExternalLink{
			ID:          uuid.NewString(),
			Key:         "docsearch",
			Label:       "ระบบสืบค้นเอกสาร (กำลังพัฒนา)",
			URL:         dsURL,
			Status:      dsStatus,
			Description: "เชื่อมต่อฐานข้อมูลคู่มือ มาตรฐานการพยาบาล และเอกสารภายใน",
			Icon:        "📂",
			SortOrder:   2,
		})
	}

	// 3. Seed starter users & sample posts only on very first launch
	var seeded models.SystemSetting
	if err := db.Where("key = ?", "initial_seeded").First(&seeded).Error; err == nil {
		return
	}

	var userCount int64
	db.Model(&models.User{}).Where("is_hidden = ?", false).Count(&userCount)
	if userCount == 0 {
		defaultHash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
		starterUsers := []models.User{
			{
				ID:           uuid.NewString(),
				Username:     "head_s2",
				DisplayName:  "หัวหน้างานพิเศษ 2",
				PasswordHash: string(defaultHash),
				Role:         "admin",
				Department:   models.DeptSpecial2,
				Ward:         "หอสงฆ์",
				IsActive:     true,
			},
			{
				ID:           uuid.NewString(),
				Username:     "head_s3",
				DisplayName:  "หัวหน้างานพิเศษ 3",
				PasswordHash: string(defaultHash),
				Role:         "admin",
				Department:   models.DeptSpecial3,
				Ward:         "8A",
				IsActive:     true,
			},
		}
		for _, u := range starterUsers {
			db.Create(&u)
		}
	}

	var postCount int64
	db.Model(&models.Post{}).Count(&postCount)
	if postCount == 0 {
		now := time.Now()
		meetingStart := now.Add(48 * time.Hour)
		meetingEnd := meetingStart.Add(2 * time.Hour)
		samplePosts := []models.Post{
			{
				ID:            uuid.NewString(),
				Category:      "award",
				ScopeDept:     models.DeptSpecial2,
				TargetWards:   `["all"]`,
				Title:         "ยินดีรางวัลผลงานพัฒนาคุณภาพ (CQI) งานผู้ป่วยพิเศษ 2",
				Content:       "ขอร่วมแสดงความยินดีกับทีมพยาบาลหอสงฆ์, สว11 และ สว12 ที่ได้รับรางวัลชมเชย CQI ประจำปี",
				IsPinned:      true,
				Priority:      "normal",
				HonoreeName:   "ทีมพัฒนาคุณภาพ พิเศษ 2",
				HonoreeWard:   "หอสงฆ์ / สว11 / สว12",
				AwardTitle:    "รางวัลชมเชย CQI ประจำปี",
				ApplauseCount: 12,
				AuthorID:      "system",
				AuthorName:    "หัวหน้างานพิเศษ 2",
				AuthorWard:    "หอสงฆ์",
				CreatedAt:     now.Add(-2 * time.Hour),
				UpdatedAt:     now.Add(-2 * time.Hour),
			},
			{
				ID:             uuid.NewString(),
				Category:       "meeting",
				ScopeDept:      models.DeptSpecial2,
				TargetWards:    `["all"]`,
				Title:          "ประชุมวิชาการและทบทวนการใช้ LearnChain (พิเศษ 2)",
				Content:        "เชิญตัวแทนหอสงฆ์, สว11, สว12 เข้าร่วมประชุมชี้แจงการเก็บชั่วโมง E-Learning",
				IsPinned:       false,
				Priority:       "normal",
				EventStartAt:   &meetingStart,
				EventEndAt:     &meetingEnd,
				LocationOrLink: "ห้องประชุมชั้น 9 / Zoom Online",
				Speaker:        "คณะกรรมการพัฒนาบุคลากร",
				AuthorID:       "system",
				AuthorName:     "หัวหน้างานพิเศษ 2",
				AuthorWard:     "สว11",
				CreatedAt:      now.Add(-1 * time.Hour),
				UpdatedAt:      now.Add(-1 * time.Hour),
			},
			{
				ID:            uuid.NewString(),
				Category:      "award",
				ScopeDept:     models.DeptSpecial3,
				TargetWards:   `["8A","8B","9A","9B","9C"]`,
				Title:         "ยินดีบุคลากรดีเด่น งานการพยาบาลผู้ป่วยพิเศษ 3",
				Content:       "ขอแสดงความยินดีกับทีมพยาบาล 8A-9C ในความมุ่งมั่นดูแลผู้ป่วยด้วยมาตรฐานวิชาชีพ",
				IsPinned:      true,
				Priority:      "normal",
				HonoreeName:   "ทีมพยาบาลพิเศษ 3",
				HonoreeWard:   "8A / 8B / 9A / 9B / 9C",
				AwardTitle:    "บุคลากรดีเด่นประจำไตรมาส",
				ApplauseCount: 15,
				AuthorID:      "system",
				AuthorName:    "หัวหน้างานพิเศษ 3",
				AuthorWard:    "8A",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		}
		for _, p := range samplePosts {
			db.Create(&p)
		}
	}

	db.Save(&models.SystemSetting{
		Key:       "initial_seeded",
		Value:     "true",
		UpdatedAt: time.Now(),
	})
}
