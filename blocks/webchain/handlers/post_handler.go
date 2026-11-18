package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type PostHandler struct{}

func NewPostHandler() *PostHandler {
	return &PostHandler{}
}

func (h *PostHandler) ListPosts(c *fiber.Ctx) error {
	dept := strings.TrimSpace(c.Query("dept", ""))
	ward := strings.TrimSpace(c.Query("ward", ""))
	category := strings.TrimSpace(c.Query("category", ""))
	includeOld := c.Query("include_old", "false") == "true"
	maxAgeDays := c.QueryInt("max_age_days", 30)
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}

	var posts []models.Post
	q := database.DB.Model(&models.Post{}).Order("is_pinned desc, created_at desc")
	if category != "" && category != "all" {
		q = q.Where("category = ?", category)
	}
	if dept != "" && dept != "all" {
		q = q.Where("scope_dept = ?", dept)
	}
	if err := q.Find(&posts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "โหลดประกาศไม่สำเร็จ"})
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -maxAgeDays)
	meetingCutoff := now.AddDate(0, 0, -7)

	visible := make([]models.Post, 0, len(posts))
	archivedCount := 0

	for _, p := range posts {
		if ward != "" && ward != "all" && !postMatchesWard(p.TargetWards, ward) {
			continue
		}
		isOld := false
		if !p.IsPinned {
			if p.CreatedAt.Before(cutoff) {
				isOld = true
			} else if p.Category == "meeting" && p.EventStartAt != nil && p.EventStartAt.Before(meetingCutoff) {
				isOld = true
			}
		}
		if isOld {
			archivedCount++
			if !includeOld {
				continue
			}
		}
		visible = append(visible, p)
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"posts":          visible,
		"archived_count": archivedCount,
	})
}

func postMatchesWard(targetWardsJSON, ward string) bool {
	if targetWardsJSON == "" || targetWardsJSON == "[]" {
		return true
	}
	var wards []string
	if err := json.Unmarshal([]byte(targetWardsJSON), &wards); err != nil {
		return true
	}
	for _, w := range wards {
		if w == "all" || strings.EqualFold(w, ward) {
			return true
		}
	}
	return false
}

func (h *PostHandler) CreatePost(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "กรุณาเข้าสู่ระบบก่อนโพสต์"})
	}

	var body struct {
		Category       string   `json:"category"`
		ScopeDept      string   `json:"scope_dept"`
		TargetWards    []string `json:"target_wards"`
		Title          string   `json:"title"`
		Content        string   `json:"content"`
		ImageData      string   `json:"image_data"`
		Images         []string `json:"images"`
		AttachmentName string   `json:"attachment_name"`
		IsPinned       bool     `json:"is_pinned"`
		Priority       string   `json:"priority"`
		HonoreeName    string   `json:"honoree_name"`
		HonoreeWard    string   `json:"honoree_ward"`
		AwardTitle     string   `json:"award_title"`
		EventStartAt   string   `json:"event_start_at"`
		EventEndAt     string   `json:"event_end_at"`
		LocationOrLink string   `json:"location_or_link"`
		Speaker        string   `json:"speaker"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลโพสต์ไม่ถูกต้อง"})
	}

	title := strings.TrimSpace(body.Title)
	if title == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณาระบุหัวข้อ"})
	}
	cat := body.Category
	if cat != "award" && cat != "meeting" && cat != "announcement" && cat != "activity" {
		cat = "announcement"
	}
	scopeDept := body.ScopeDept
	if scopeDept == "" {
		scopeDept = models.DeptAll
	}
	wards := body.TargetWards
	if len(wards) == 0 {
		wards = []string{"all"}
	}
	wardsJSON, _ := json.Marshal(wards)

	priority := body.Priority
	if priority != "urgent" {
		priority = "normal"
	}

	imagesList := body.Images
	if len(imagesList) > 5 {
		imagesList = imagesList[:5]
	}
	coverImg := body.ImageData
	if coverImg == "" && len(imagesList) > 0 {
		coverImg = imagesList[0]
	}
	imagesJSON, _ := json.Marshal(imagesList)

	now := time.Now()
	post := models.Post{
		ID:             uuid.NewString(),
		Category:       cat,
		ScopeDept:      scopeDept,
		TargetWards:    string(wardsJSON),
		Title:          title,
		Content:        strings.TrimSpace(body.Content),
		ImageData:      coverImg,
		Images:         string(imagesJSON),
		AttachmentName: body.AttachmentName,
		IsPinned:       body.IsPinned,
		Priority:       priority,
		HonoreeName:    strings.TrimSpace(body.HonoreeName),
		HonoreeWard:    strings.TrimSpace(body.HonoreeWard),
		AwardTitle:     strings.TrimSpace(body.AwardTitle),
		LocationOrLink: strings.TrimSpace(body.LocationOrLink),
		Speaker:        strings.TrimSpace(body.Speaker),
		AuthorID:       user.ID,
		AuthorName:     user.DisplayName,
		AuthorWard:     user.Ward,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if body.EventStartAt != "" {
		if t, err := parseFlexibleTime(body.EventStartAt); err == nil {
			post.EventStartAt = &t
		}
	}
	if body.EventEndAt != "" {
		if t, err := parseFlexibleTime(body.EventEndAt); err == nil {
			post.EventEndAt = &t
		}
	}

	if err := database.DB.Create(&post).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "บันทึกโพสต์ไม่สำเร็จ"})
	}
	if GlobalSyncHandler != nil {
		GlobalSyncHandler.TriggerImmediateSync()
	}

	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: user.DisplayName,
		Action:    "create_post",
		Target:    post.ID,
		Detail:    "[" + post.Category + "] " + post.Title,
		CreatedAt: now,
	})

	return c.JSON(fiber.Map{"success": true, "post": post})
}

func (h *PostHandler) UpdatePost(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "กรุณาเข้าสู่ระบบ"})
	}
	id := c.Params("id")
	var post models.Post
	if err := database.DB.Where("id = ?", id).First(&post).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบโพสต์"})
	}
	if post.AuthorID != user.ID && user.Role != "admin" && user.Role != "god" {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "คุณแก้ไขได้เฉพาะโพสต์ของตนเอง"})
	}

	var body struct {
		Category       *string  `json:"category"`
		Title          *string  `json:"title"`
		Content        *string  `json:"content"`
		ScopeDept      *string  `json:"scope_dept"`
		TargetWards    []string `json:"target_wards"`
		IsPinned       *bool    `json:"is_pinned"`
		Priority       *string  `json:"priority"`
		HonoreeName    *string  `json:"honoree_name"`
		HonoreeWard    *string  `json:"honoree_ward"`
		AwardTitle     *string  `json:"award_title"`
		EventStartAt   *string  `json:"event_start_at"`
		EventEndAt     *string  `json:"event_end_at"`
		LocationOrLink *string  `json:"location_or_link"`
		Speaker        *string  `json:"speaker"`
		ImageData      *string  `json:"image_data"`
		Images         []string `json:"images"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}
	if body.Category != nil {
		cat := *body.Category
		if cat == "award" || cat == "meeting" || cat == "activity" || cat == "announcement" {
			post.Category = cat
		}
	}
	if body.EventStartAt != nil {
		if *body.EventStartAt == "" {
			post.EventStartAt = nil
		} else if t, err := parseFlexibleTime(*body.EventStartAt); err == nil {
			post.EventStartAt = &t
		}
	}
	if body.EventEndAt != nil {
		if *body.EventEndAt == "" {
			post.EventEndAt = nil
		} else if t, err := parseFlexibleTime(*body.EventEndAt); err == nil {
			post.EventEndAt = &t
		}
	}
	if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
		post.Title = strings.TrimSpace(*body.Title)
	}
	if body.Content != nil {
		post.Content = strings.TrimSpace(*body.Content)
	}
	if body.ScopeDept != nil {
		post.ScopeDept = *body.ScopeDept
	}
	if len(body.TargetWards) > 0 {
		b, _ := json.Marshal(body.TargetWards)
		post.TargetWards = string(b)
	}
	if body.IsPinned != nil {
		post.IsPinned = *body.IsPinned
	}
	if body.Priority != nil {
		post.Priority = *body.Priority
	}
	if body.HonoreeName != nil {
		post.HonoreeName = *body.HonoreeName
	}
	if body.HonoreeWard != nil {
		post.HonoreeWard = *body.HonoreeWard
	}
	if body.AwardTitle != nil {
		post.AwardTitle = *body.AwardTitle
	}
	if body.LocationOrLink != nil {
		post.LocationOrLink = *body.LocationOrLink
	}
	if body.Speaker != nil {
		post.Speaker = *body.Speaker
	}
	if body.ImageData != nil {
		post.ImageData = *body.ImageData
	}
	if body.Images != nil {
		imgs := body.Images
		if len(imgs) > 5 {
			imgs = imgs[:5]
		}
		b, _ := json.Marshal(imgs)
		post.Images = string(b)
		if len(imgs) > 0 {
			post.ImageData = imgs[0]
		}
	}
	post.UpdatedAt = time.Now()
	database.DB.Save(&post)
	if GlobalSyncHandler != nil {
		GlobalSyncHandler.TriggerImmediateSync()
	}

	return c.JSON(fiber.Map{"success": true, "post": post})
}

func (h *PostHandler) DeletePost(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "กรุณาเข้าสู่ระบบ"})
	}
	id := c.Params("id")
	var post models.Post
	if err := database.DB.Where("id = ?", id).First(&post).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบโพสต์"})
	}
	if post.AuthorID != user.ID && user.Role != "admin" && user.Role != "god" {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "คุณลบได้เฉพาะโพสต์ของตนเอง"})
	}

	database.DB.Delete(&post)
	if GlobalSyncHandler != nil {
		GlobalSyncHandler.TriggerImmediateSync()
	}
	database.DB.Create(&models.AuditLog{
		ID:        uuid.NewString(),
		ActorName: user.DisplayName,
		Action:    "delete_post",
		Target:    post.ID,
		Detail:    post.Title,
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{"success": true, "message": "ลบประกาศเรียบร้อย"})
}

func (h *PostHandler) ApplausePost(c *fiber.Ctx) error {
	id := c.Params("id")
	var post models.Post
	if err := database.DB.Where("id = ?", id).First(&post).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "ไม่พบโพสต์"})
	}
	post.ApplauseCount++
	post.UpdatedAt = time.Now()
	database.DB.Model(&post).Updates(map[string]interface{}{
		"applause_count": post.ApplauseCount,
		"updated_at":     post.UpdatedAt,
	})
	return c.JSON(fiber.Map{"success": true, "applause_count": post.ApplauseCount})
}

// GetStats returns comprehensive post statistics & top contributors
func (h *PostHandler) GetStats(c *fiber.Ctx) error {
	var posts []models.Post
	database.DB.Find(&posts)

	byCategory := map[string]int{"award": 0, "meeting": 0, "activity": 0, "announcement": 0}
	byDept := map[string]int{"all": 0, "special-2": 0, "special-3": 0}
	byWard := map[string]int{}
	totalApplause := 0

	type AuthorStat struct {
		AuthorID      string `json:"author_id"`
		AuthorName    string `json:"author_name"`
		AuthorWard    string `json:"author_ward"`
		PostCount     int    `json:"post_count"`
		AwardCount    int    `json:"award_count"`
		MeetingCount  int    `json:"meeting_count"`
		AnnounceCount int    `json:"announce_count"`
		ApplauseTotal int    `json:"applause_total"`
	}
	authorMap := map[string]*AuthorStat{}

	for _, p := range posts {
		byCategory[p.Category]++
		byDept[p.ScopeDept]++
		totalApplause += p.ApplauseCount

		var wards []string
		if err := json.Unmarshal([]byte(p.TargetWards), &wards); err == nil {
			for _, w := range wards {
				byWard[w]++
			}
		}

		aKey := p.AuthorName
		if p.AuthorID != "" {
			aKey = p.AuthorID
		}
		if _, exists := authorMap[aKey]; !exists {
			authorMap[aKey] = &AuthorStat{
				AuthorID:   p.AuthorID,
				AuthorName: p.AuthorName,
				AuthorWard: p.AuthorWard,
			}
		}
		st := authorMap[aKey]
		st.PostCount++
		st.ApplauseTotal += p.ApplauseCount
		switch p.Category {
		case "award":
			st.AwardCount++
		case "meeting":
			st.MeetingCount++
		default:
			st.AnnounceCount++
		}
	}

	authors := make([]AuthorStat, 0, len(authorMap))
	for _, v := range authorMap {
		authors = append(authors, *v)
	}
	// Sort descending by PostCount
	for i := 0; i < len(authors); i++ {
		for j := i + 1; j < len(authors); j++ {
			if authors[j].PostCount > authors[i].PostCount {
				authors[i], authors[j] = authors[j], authors[i]
			}
		}
	}

	var visibleUsers int64
	database.DB.Model(&models.User{}).Where("is_hidden = ? AND is_active = ?", false, true).Count(&visibleUsers)

	return c.JSON(fiber.Map{
		"success":        true,
		"total_posts":    len(posts),
		"total_applause": totalApplause,
		"total_users":    visibleUsers,
		"by_category":    byCategory,
		"by_dept":        byDept,
		"by_ward":        byWard,
		"top_authors":    authors,
	})
}

func parseFlexibleTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fiber.ErrBadRequest
}
