package handlers

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type WardHandler struct{}

func NewWardHandler() *WardHandler {
	return &WardHandler{}
}

func (h *WardHandler) ListWards(c *fiber.Ctx) error {
	dept := strings.TrimSpace(c.Query("dept", ""))
	var wards []models.WardProfile
	q := database.DB.Model(&models.WardProfile{})
	if dept != "" && dept != "all" {
		q = q.Where("department = ?", dept)
	}
	q.Find(&wards)
	return c.JSON(fiber.Map{"success": true, "wards": wards})
}

func (h *WardHandler) GetWard(c *fiber.Ctx) error {
	code, _ := url.PathUnescape(c.Params("code"))
	var ward models.WardProfile
	if err := database.DB.Where("ward_code = ?", code).First(&ward).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบข้อมูลหอผู้ป่วย"})
	}
	return c.JSON(fiber.Map{"success": true, "ward": ward})
}

func (h *WardHandler) UpdateWard(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "กรุณาเข้าสู่ระบบ"})
	}

	code, _ := url.PathUnescape(c.Params("code"))
	var ward models.WardProfile
	if err := database.DB.Where("ward_code = ?", code).First(&ward).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบข้อมูลหอผู้ป่วย"})
	}

	var body struct {
		DisplayName   string          `json:"display_name"`
		CoverImage    *string         `json:"cover_image"`
		LeaderName    string          `json:"leader_name"`
		LeaderTitle   string          `json:"leader_title"`
		LeaderPhoto   *string         `json:"leader_photo"`
		History       string          `json:"history"`
		BedCount      int             `json:"bed_count"`
		PatientGroups string          `json:"patient_groups"`
		StaffLevels   json.RawMessage `json:"staff_levels"`
		Roster        json.RawMessage `json:"roster"`
		BasicStats    json.RawMessage `json:"basic_stats"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}

	if strings.TrimSpace(body.DisplayName) != "" {
		ward.DisplayName = strings.TrimSpace(body.DisplayName)
	}
	if body.CoverImage != nil {
		ward.CoverImage = *body.CoverImage
	}
	ward.LeaderName = strings.TrimSpace(body.LeaderName)
	ward.LeaderTitle = strings.TrimSpace(body.LeaderTitle)
	if body.LeaderPhoto != nil {
		ward.LeaderPhoto = *body.LeaderPhoto
	}
	ward.History = strings.TrimSpace(body.History)
	if body.BedCount >= 0 {
		ward.BedCount = body.BedCount
	}
	ward.PatientGroups = strings.TrimSpace(body.PatientGroups)
	if len(body.StaffLevels) > 0 {
		ward.StaffLevels = string(body.StaffLevels)
	}
	if len(body.Roster) > 0 {
		ward.Roster = string(body.Roster)
	}
	if len(body.BasicStats) > 0 {
		ward.BasicStats = string(body.BasicStats)
	}
	ward.UpdatedAt = time.Now()
	database.DB.Save(&ward)

	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: user.DisplayName,
		Action:    "update_ward",
		Target:    ward.WardCode,
		Detail:    "อัปเดตข้อมูลหอผู้ป่วย " + ward.DisplayName,
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{"success": true, "ward": ward})
}

type RosterMember struct {
	Name         string `json:"name"`
	Position     string `json:"position"`
	Level        string `json:"level"`
	PhotoData    string `json:"photo_data"`
	PhotoAltData string `json:"photo_alt_data,omitempty"`
	PhotoPosX    *int    `json:"photo_pos_x,omitempty"`
	PhotoPosY    *int    `json:"photo_pos_y,omitempty"`
	PhotoFlip    bool    `json:"photo_flip,omitempty"`
	PhotoZoom    float64 `json:"photo_zoom,omitempty"`
	PhotoAltPosX *int    `json:"photo_alt_pos_x,omitempty"`
	PhotoAltPosY *int    `json:"photo_alt_pos_y,omitempty"`
	PhotoAltFlip bool    `json:"photo_alt_flip,omitempty"`
	PhotoAltZoom float64 `json:"photo_alt_zoom,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Email      string `json:"email,omitempty"`
	BirthMonth int    `json:"birth_month"`
	BirthDay   int    `json:"birth_day"`
}

type MonthlyBirthdayItem struct {
	Department string `json:"department"`
	WardCode   string `json:"ward_code"`
	Name       string `json:"name"`
	Position   string `json:"position"`
	PhotoData  string `json:"photo_data,omitempty"`
	BirthMonth int    `json:"birth_month"`
	BirthDay   int    `json:"birth_day"`
	IsToday    bool   `json:"is_today"`
	WishCount  int    `json:"wish_count"`
}

func (h *WardHandler) GetMonthlyBirthdays(c *fiber.Ctx) error {
	dept := strings.TrimSpace(c.Query("dept", ""))
	now := time.Now()
	month := c.QueryInt("month", int(now.Month()))
	if month < 1 || month > 12 {
		month = int(now.Month())
	}
	yearMonth := now.Format("2006-") + padTwo(month)

	var wards []models.WardProfile
	q := database.DB.Model(&models.WardProfile{})
	if dept != "" && dept != "all" {
		q = q.Where("department = ?", dept)
	}
	q.Find(&wards)

	var wishes []models.BirthdayWish
	database.DB.Where("year_month = ?", yearMonth).Find(&wishes)
	wishMap := map[string]int{}
	for _, w := range wishes {
		wishMap[w.ID] = w.WishCount
	}

	items := make([]MonthlyBirthdayItem, 0)
	for _, w := range wards {
		var roster []RosterMember
		if err := json.Unmarshal([]byte(w.Roster), &roster); err != nil {
			continue
		}
		for _, m := range roster {
			if m.BirthMonth == month && strings.TrimSpace(m.Name) != "" {
				wishID := w.Department + "|" + w.WardCode + "|" + m.Name + "|" + yearMonth
				items = append(items, MonthlyBirthdayItem{
					Department: w.Department,
					WardCode:   w.WardCode,
					Name:       m.Name,
					Position:   m.Position,
					PhotoData:  m.PhotoData,
					BirthMonth: m.BirthMonth,
					BirthDay:   m.BirthDay,
					IsToday:    m.BirthMonth == int(now.Month()) && m.BirthDay == now.Day(),
					WishCount:  wishMap[wishID],
				})
			}
		}
	}

	// Sort by birth_day ascending
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].BirthDay < items[i].BirthDay {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"month":      month,
		"year_month": yearMonth,
		"birthdays":  items,
	})
}

func (h *WardHandler) SendBirthdayWish(c *fiber.Ctx) error {
	var body struct {
		Department string `json:"department"`
		WardCode   string `json:"ward_code"`
		MemberName string `json:"member_name"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.MemberName) == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}
	now := time.Now()
	yearMonth := now.Format("2006-01")
	wishID := body.Department + "|" + body.WardCode + "|" + body.MemberName + "|" + yearMonth

	var wish models.BirthdayWish
	err := database.DB.Where("id = ?", wishID).First(&wish).Error
	if err != nil {
		wish = models.BirthdayWish{
			ID:         wishID,
			Department: body.Department,
			WardCode:   body.WardCode,
			MemberName: body.MemberName,
			YearMonth:  yearMonth,
			WishCount:  1,
			UpdatedAt:  now,
		}
		database.DB.Create(&wish)
	} else {
		wish.WishCount++
		wish.UpdatedAt = now
		database.DB.Save(&wish)
	}

	return c.JSON(fiber.Map{"success": true, "wish_count": wish.WishCount})
}

func padTwo(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
