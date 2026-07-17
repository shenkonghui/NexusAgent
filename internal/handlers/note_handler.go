package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
	"nexusagent/internal/services"
)

var noteTagRe = regexp.MustCompile(`#([^\s#]+)`)

// NoteHandler 处理笔记 CRUD。
type NoteHandler struct {
	repo         *repository.NoteRepository
	settingsRepo *repository.NoteSettingsRepository
	classifier   *services.NoteClassifier
}

func NewNoteHandler(
	repo *repository.NoteRepository,
	settingsRepo *repository.NoteSettingsRepository,
	classifier *services.NoteClassifier,
) *NoteHandler {
	return &NoteHandler{repo: repo, settingsRepo: settingsRepo, classifier: classifier}
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
	McpToken                string `json:"mcp_token"`
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

func parseNoteTags(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}
	var tags []string
	for _, m := range noteTagRe.FindAllStringSubmatch(lines[0], -1) {
		if len(m) > 1 {
			tags = append(tags, m[1])
		}
	}
	return tags
}

type noteFrontmatter struct {
	Title string   `yaml:"title"`
	Tags  []string `yaml:"tags"`
}

func formatNoteMarkdown(title, content string, tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	meta, err := yaml.Marshal(noteFrontmatter{Title: title, Tags: tags})
	if err != nil {
		return content
	}
	return "---\n" + string(meta) + "---\n" + content
}

func parseNoteMarkdown(raw string) (title, content string, tags []string) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---") {
		return "", raw, nil
	}
	rest := strings.TrimPrefix(raw, "---")
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", raw, nil
	}
	fm := rest[:idx]
	body := strings.TrimLeft(rest[idx+len("\n---"):], "\r\n")
	var meta noteFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return "", raw, nil
	}
	return strings.TrimSpace(meta.Title), body, meta.Tags
}

func mergeNoteTags(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, t := range append(append([]string{}, a...), b...) {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out
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
	Success(c, http.StatusOK, toNoteSettingsItem(s, services.EffectiveClassifyPrompt(s.ClassifyPrompt)))
}

// GenerateMCPToken POST /api/v1/notes/settings/mcp-token
func (h *NoteHandler) GenerateMCPToken(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "生成 token 失败")
		return
	}
	token := hex.EncodeToString(b)
	if err := h.settingsRepo.SetMCPTokenOnce(uid, token); err != nil {
		if errors.Is(err, repository.ErrMCPTokenAlreadySet) {
			Fail(c, http.StatusConflict, "MCP_TOKEN_EXISTS", "MCP Token 已生成，不可重复生成")
			return
		}
		Fail(c, http.StatusInternalServerError, "INTERNAL", "保存 MCP Token 失败")
		return
	}
	Success(c, http.StatusOK, gin.H{"mcp_token": token})
}

func toNoteSettingsItem(s *models.NoteSettings, prompt string) noteSettingsItem {
	return noteSettingsItem{
		AgentType:               s.AgentType,
		ModelValue:              s.ModelValue,
		ClassifyPrompt:          prompt,
		ClassifyIntervalMinutes: services.NormalizeClassifyIntervalMinutes(s.ClassifyIntervalMinutes),
		ClassifySessionID:       s.ClassifySessionID,
		ClassifyDBSessionID:     s.ClassifyDBSessionID,
		McpToken:                s.McpToken,
	}
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
	Success(c, http.StatusOK, toNoteSettingsItem(saved, services.EffectiveClassifyPrompt(saved.ClassifyPrompt)))
}

// List GET /api/v1/notes?tag=xxx&q=xxx&page=1&limit=20
func (h *NoteHandler) List(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	tag := strings.TrimSpace(c.Query("tag"))
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, total, err := h.repo.FindByUserIDPaged(uid, tag, q, page, limit)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询笔记失败")
		return
	}
	items := make([]noteItem, 0, len(list))
	for i := range list {
		items = append(items, toNoteItem(&list[i]))
	}
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	Success(c, http.StatusOK, gin.H{
		"notes": items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
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
	tags := parseNoteTags(content)
	n := &models.Note{
		UserID:          uid,
		Title:           "",
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
	tags := parseNoteTags(content)
	n.Content = content
	n.Tags = tagsToJSON(tags)
	if n.Title == "" && h.shouldEnqueueClassify(n.UserID) {
		n.ClassifyPending = true
	}
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

// ClassifyNow POST /api/v1/notes/:id/classify — 立即分类（忽略间隔）。
func (h *NoteHandler) ClassifyNow(c *gin.Context) {
	n, ok := h.loadOwnedNote(c)
	if !ok {
		return
	}
	if h.classifier == nil {
		Fail(c, http.StatusServiceUnavailable, "UNAVAILABLE", "分类服务未就绪")
		return
	}
	if !h.shouldEnqueueClassify(n.UserID) {
		Fail(c, http.StatusBadRequest, "AGENT_REQUIRED", "请先在笔记设置中配置分类 Agent")
		return
	}
	updated, err := h.classifier.ClassifyNow(c.Request.Context(), n.UserID, n.ID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "CLASSIFY_FAILED", err.Error())
		return
	}
	Success(c, http.StatusOK, toNoteItem(updated))
}

type noteImportRequest struct {
	Notes []noteImportItem `json:"notes" binding:"required"`
}

type noteImportItem struct {
	Content string   `json:"content"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
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
		if content == "" {
			continue
		}
		parts = append(parts, formatNoteMarkdown(list[i].Title, content, tagsFromJSON(list[i].Tags)))
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
		title := strings.TrimSpace(item.Title)
		tags := mergeNoteTags(item.Tags, parseNoteTags(content))
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
