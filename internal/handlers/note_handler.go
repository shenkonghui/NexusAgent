package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
	"nexusagent/internal/services"
)

var noteTagRe = regexp.MustCompile(`#([^\s#]+)`)

// NoteHandler 处理笔记 CRUD。
type NoteHandler struct {
	repo         *repository.NoteRepository
	settingsRepo *repository.NoteSettingsRepository
}

func NewNoteHandler(
	repo *repository.NoteRepository,
	settingsRepo *repository.NoteSettingsRepository,
) *NoteHandler {
	return &NoteHandler{repo: repo, settingsRepo: settingsRepo}
}

type noteItem struct {
	ID              uint     `json:"id"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	Tags            []string `json:"tags"`
	ClassifyPending bool     `json:"classify_pending"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type noteRequest struct {
	Content string `json:"content" binding:"required"`
}

type noteSettingsItem struct {
	AgentType               string `json:"agent_type"`
	ModelValue              string `json:"model_value"`
	ClassifyPrompt          string `json:"classify_prompt"`
	ClassifyIntervalMinutes int    `json:"classify_interval_minutes"`
	ClassifySessionID       string `json:"classify_session_id"`
	ClassifyDBSessionID     uint   `json:"classify_db_session_id"`
}

type noteSettingsRequest struct {
	AgentType               string `json:"agent_type"`
	ModelValue              string `json:"model_value"`
	ClassifyPrompt          string `json:"classify_prompt"`
	ClassifyIntervalMinutes int    `json:"classify_interval_minutes"`
}

func parseNoteID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的笔记 ID")
		return 0, false
	}
	return uint(id), true
}

func parseNoteMeta(content string) (title string, tags []string) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "无标题", nil
	}
	first := lines[0]
	for _, m := range noteTagRe.FindAllStringSubmatch(first, -1) {
		if len(m) > 1 {
			tags = append(tags, m[1])
		}
	}
	titlePart := strings.TrimSpace(noteTagRe.ReplaceAllString(first, ""))
	if titlePart != "" {
		return truncateTitle(titlePart), tags
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncateTitle(line), tags
		}
	}
	return "无标题", tags
}

func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= 80 {
		return s
	}
	runes := []rune(s)
	return string(runes[:80]) + "…"
}

func tagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func tagsFromJSON(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}
	}
	return tags
}

func toNoteItem(n *models.Note) noteItem {
	return noteItem{
		ID:              n.ID,
		Title:           n.Title,
		Content:         n.Content,
		Tags:            tagsFromJSON(n.Tags),
		ClassifyPending: n.ClassifyPending,
		CreatedAt:       n.CreatedAt.Format(timeRFC3339),
		UpdatedAt:       n.UpdatedAt.Format(timeRFC3339),
	}
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func (h *NoteHandler) loadOwnedNote(c *gin.Context) (*models.Note, bool) {
	id, ok := parseNoteID(c)
	if !ok {
		return nil, false
	}
	n, err := h.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNoteNotFound) {
			Fail(c, http.StatusNotFound, "NOT_FOUND", "笔记不存在")
		} else {
			Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记失败")
		}
		return nil, false
	}
	uid, ok := currentUserID(c)
	if !ok || n.UserID != uid {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "笔记不存在")
		return nil, false
	}
	return n, true
}

func (h *NoteHandler) shouldEnqueueClassify(uid uint) bool {
	s, err := h.settingsRepo.FindByUserID(uid)
	if err != nil {
		return false
	}
	return strings.TrimSpace(s.AgentType) != ""
}

// GetSettings GET /api/v1/notes/settings
func (h *NoteHandler) GetSettings(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	s, err := h.settingsRepo.FindByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记设置失败")
		return
	}
	prompt := s.ClassifyPrompt
	if prompt == "" {
		prompt = services.DefaultNoteClassifyPrompt
	}
	Success(c, http.StatusOK, noteSettingsItem{
		AgentType:               s.AgentType,
		ModelValue:              s.ModelValue,
		ClassifyPrompt:          prompt,
		ClassifyIntervalMinutes: services.NormalizeClassifyIntervalMinutes(s.ClassifyIntervalMinutes),
		ClassifySessionID:       s.ClassifySessionID,
		ClassifyDBSessionID:     s.ClassifyDBSessionID,
	})
}

// UpdateSettings PUT /api/v1/notes/settings
func (h *NoteHandler) UpdateSettings(c *gin.Context) {
	var req noteSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	s := &models.NoteSettings{
		UserID:                  uid,
		AgentType:               strings.TrimSpace(req.AgentType),
		ModelValue:              strings.TrimSpace(req.ModelValue),
		ClassifyPrompt:          strings.TrimSpace(req.ClassifyPrompt),
		ClassifyIntervalMinutes: services.NormalizeClassifyIntervalMinutes(req.ClassifyIntervalMinutes),
	}
	if err := h.settingsRepo.Upsert(s); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "保存笔记设置失败")
		return
	}
	saved, err := h.settingsRepo.FindByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记设置失败")
		return
	}
	prompt := saved.ClassifyPrompt
	if prompt == "" {
		prompt = services.DefaultNoteClassifyPrompt
	}
	Success(c, http.StatusOK, noteSettingsItem{
		AgentType:               saved.AgentType,
		ModelValue:              saved.ModelValue,
		ClassifyPrompt:          prompt,
		ClassifyIntervalMinutes: services.NormalizeClassifyIntervalMinutes(saved.ClassifyIntervalMinutes),
		ClassifySessionID:       saved.ClassifySessionID,
		ClassifyDBSessionID:     saved.ClassifyDBSessionID,
	})
}

// List GET /api/v1/notes?tag=xxx
func (h *NoteHandler) List(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	tag := strings.TrimSpace(c.Query("tag"))
	list, err := h.repo.FindByUserID(uid, tag)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记失败")
		return
	}
	items := make([]noteItem, 0, len(list))
	for i := range list {
		items = append(items, toNoteItem(&list[i]))
	}
	Success(c, http.StatusOK, gin.H{"notes": items})
}

// ListTags GET /api/v1/notes/tags
func (h *NoteHandler) ListTags(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	tags, err := h.repo.ListTags(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询标签失败")
		return
	}
	if tags == nil {
		tags = []string{}
	}
	Success(c, http.StatusOK, gin.H{"tags": tags})
}

// Create POST /api/v1/notes
func (h *NoteHandler) Create(c *gin.Context) {
	var req noteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "内容不能为空")
		return
	}
	title, tags := parseNoteMeta(content)
	n := &models.Note{
		UserID:          uid,
		Title:           title,
		Content:         content,
		Tags:            tagsToJSON(tags),
		ClassifyPending: h.shouldEnqueueClassify(uid),
	}
	if err := h.repo.Create(n); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "创建笔记失败")
		return
	}
	Success(c, http.StatusCreated, toNoteItem(n))
}

// Get GET /api/v1/notes/:id
func (h *NoteHandler) Get(c *gin.Context) {
	n, ok := h.loadOwnedNote(c)
	if !ok {
		return
	}
	Success(c, http.StatusOK, toNoteItem(n))
}

// Update PUT /api/v1/notes/:id
func (h *NoteHandler) Update(c *gin.Context) {
	n, ok := h.loadOwnedNote(c)
	if !ok {
		return
	}
	var req noteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "内容不能为空")
		return
	}
	title, tags := parseNoteMeta(content)
	n.Title = title
	n.Content = content
	n.Tags = tagsToJSON(tags)
	if err := h.repo.Update(n); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "更新笔记失败")
		return
	}
	Success(c, http.StatusOK, toNoteItem(n))
}

// Delete DELETE /api/v1/notes/:id
func (h *NoteHandler) Delete(c *gin.Context) {
	n, ok := h.loadOwnedNote(c)
	if !ok {
		return
	}
	if err := h.repo.Delete(n.ID); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "删除笔记失败")
		return
	}
	c.Status(http.StatusNoContent)
}

type noteImportRequest struct {
	Notes []noteImportItem `json:"notes" binding:"required"`
}

type noteImportItem struct {
	Content string `json:"content"`
}

type noteImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// Export GET /api/v1/notes/export
// 将用户全部笔记导出为单个 Markdown 文件，笔记间以独立成行的 === 分隔。
func (h *NoteHandler) Export(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	list, err := h.repo.FindAllByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记失败")
		return
	}

	parts := make([]string, 0, len(list))
	for i := range list {
		content := strings.TrimSpace(list[i].Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	body := strings.Join(parts, "\n\n===\n\n")

	filename := fmt.Sprintf("notes-%s.md", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.String(http.StatusOK, body)
}

// Import POST /api/v1/notes/import
// 批量导入笔记，按内容（去空白后）去重。
func (h *NoteHandler) Import(c *gin.Context) {
	var req noteImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}

	// 构建已有内容集合用于去重
	existing, err := h.repo.FindAllByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记失败")
		return
	}
	seen := make(map[string]struct{}, len(existing))
	for i := range existing {
		key := strings.TrimSpace(existing[i].Content)
		if key != "" {
			seen[key] = struct{}{}
		}
	}

	enqueueClassify := h.shouldEnqueueClassify(uid)
	var toCreate []*models.Note
	skipped := 0
	for _, item := range req.Notes {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			skipped++
			continue
		}
		if _, dup := seen[content]; dup {
			skipped++
			continue
		}
		seen[content] = struct{}{}
		title, tags := parseNoteMeta(content)
		toCreate = append(toCreate, &models.Note{
			UserID:          uid,
			Title:           title,
			Content:         content,
			Tags:            tagsToJSON(tags),
			ClassifyPending: enqueueClassify,
		})
	}

	if len(toCreate) > 0 {
		if err := h.repo.CreateBatch(toCreate); err != nil {
			Fail(c, http.StatusInternalServerError, "INTERNAL", "导入笔记失败")
			return
		}
	}

	Success(c, http.StatusOK, noteImportResult{
		Imported: len(toCreate),
		Skipped:  skipped,
	})
}
