package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// DefaultNoteClassifyPrompt 是笔记自动分类的默认提示词模板。
const DefaultNoteClassifyPrompt = `你是一个笔记分类助手。根据笔记内容，从已有标签中选择或创建合适的新标签（小写英文或中文，不含空格和 #）。
已有标签：{{existing_tags}}
笔记内容：
{{content}}

仅输出 JSON 数组，例如 ["工作","想法"]，不要输出其他任何文字。`

const ClassifySessionTitle = "笔记分类"

const (
	DefaultClassifyIntervalMinutes = 5
	MaxClassifyIntervalMinutes     = 1440
)

// NormalizeClassifyIntervalMinutes 规范化分类间隔（分钟）。
func NormalizeClassifyIntervalMinutes(minutes int) int {
	if minutes <= 0 {
		return DefaultClassifyIntervalMinutes
	}
	if minutes > MaxClassifyIntervalMinutes {
		return MaxClassifyIntervalMinutes
	}
	return minutes
}

// ClassifyExecutor 在默认工作区会话中执行分类 prompt 并持久化消息。
type ClassifyExecutor interface {
	CreateSessionWithSource(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string) (*models.Session, error)
	GetSessionByDBID(id uint) (*models.Session, error)
	ResumeSession(ctx context.Context, sessionID string) (*models.Session, error)
	PromptWithExecution(ctx context.Context, sessionID, prompt string, executionID *uint) (<-chan models.Message, error)
	NextExecutionID(sessionID string) (uint, error)
	UpdateTitle(dbSessionID uint, title string) error
}

// NoteClassifier 根据用户配置调用 agent 为笔记打标签。
type NoteClassifier struct {
	settingsRepo *repository.NoteSettingsRepository
	noteRepo     *repository.NoteRepository
	executor     ClassifyExecutor
}

func NewNoteClassifier(
	settingsRepo *repository.NoteSettingsRepository,
	noteRepo *repository.NoteRepository,
	executor ClassifyExecutor,
) *NoteClassifier {
	return &NoteClassifier{settingsRepo: settingsRepo, noteRepo: noteRepo, executor: executor}
}

// ProcessPending 处理一批待分类笔记，返回成功处理数量。
func (c *NoteClassifier) ProcessPending(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 20
	}
	notes, err := c.noteRepo.FindPendingClassify(limit * 5)
	if err != nil {
		return 0, err
	}
	if len(notes) == 0 {
		return 0, nil
	}
	settingsCache := map[uint]int{}
	now := time.Now()
	done := 0
	for i := range notes {
		if done >= limit {
			break
		}
		interval, err := c.userClassifyInterval(settingsCache, notes[i].UserID)
		if err != nil {
			log.Printf("笔记 %d 读取分类设置失败: %v", notes[i].ID, err)
			continue
		}
		if now.Sub(notes[i].UpdatedAt) < time.Duration(interval)*time.Minute {
			continue
		}
		if err := c.classifyNote(ctx, &notes[i]); err != nil {
			log.Printf("笔记 %d 分类失败: %v", notes[i].ID, err)
			continue
		}
		done++
	}
	return done, nil
}

func (c *NoteClassifier) userClassifyInterval(cache map[uint]int, userID uint) (int, error) {
	if interval, ok := cache[userID]; ok {
		return interval, nil
	}
	settings, err := c.settingsRepo.FindByUserID(userID)
	if err != nil {
		return DefaultClassifyIntervalMinutes, err
	}
	interval := NormalizeClassifyIntervalMinutes(settings.ClassifyIntervalMinutes)
	cache[userID] = interval
	return interval, nil
}

func (c *NoteClassifier) classifyNote(ctx context.Context, note *models.Note) error {
	manualTags := tagsFromJSON(note.Tags)
	tags, err := c.classifyTags(ctx, note.UserID, note.ID, note.Content, manualTags)
	if err != nil {
		return err
	}
	note.Tags = tagsToJSON(tags)
	note.ClassifyPending = false
	return c.noteRepo.Update(note)
}

func (c *NoteClassifier) classifyTags(ctx context.Context, userID, noteID uint, content string, manualTags []string) ([]string, error) {
	settings, err := c.settingsRepo.FindByUserID(userID)
	if err != nil {
		return manualTags, err
	}
	agentType := strings.TrimSpace(settings.AgentType)
	if agentType == "" || c.executor == nil {
		return manualTags, nil
	}
	promptTpl := strings.TrimSpace(settings.ClassifyPrompt)
	if promptTpl == "" {
		promptTpl = DefaultNoteClassifyPrompt
	}
	existing, err := c.noteRepo.ListTags(userID)
	if err != nil {
		return manualTags, err
	}
	prompt := buildClassifyPrompt(promptTpl, content, existing)
	prompt = fmt.Sprintf("【笔记 #%d 自动分类】\n%s", noteID, prompt)

	session, err := c.ensureClassifySession(ctx, userID, agentType, settings)
	if err != nil {
		return manualTags, err
	}
	execID, err := c.executor.NextExecutionID(session.SessionID)
	if err != nil {
		return manualTags, err
	}
	ch, err := c.executor.PromptWithExecution(ctx, session.SessionID, prompt, &execID)
	if err != nil {
		return manualTags, err
	}
	text := collectAssistantText(ch)
	if text == "" {
		return manualTags, fmt.Errorf("agent 未返回分类结果")
	}
	autoTags := parseClassifyTags(text)
	return mergeTags(manualTags, autoTags), nil
}

func (c *NoteClassifier) ensureClassifySession(ctx context.Context, userID uint, agentType string, settings *models.NoteSettings) (*models.Session, error) {
	if settings.ClassifyDBSessionID > 0 {
		session, err := c.executor.GetSessionByDBID(settings.ClassifyDBSessionID)
		if err == nil && session.AgentType == agentType {
			if session.Status != models.SessionStatusActive {
				oldSessionID := session.SessionID
				session, err = c.executor.ResumeSession(ctx, session.SessionID)
				if err != nil {
					return nil, err
				}
				if session.SessionID != oldSessionID {
					_ = c.settingsRepo.UpdateSessionRef(userID, session.SessionID, session.ID)
				}
			}
			return session, nil
		}
	}
	session, err := c.executor.CreateSessionWithSource(ctx, agentType, 0, userID, models.SessionSourceClassify, settings.ModelValue)
	if err != nil {
		return nil, err
	}
	_ = c.executor.UpdateTitle(session.ID, ClassifySessionTitle)
	if err := c.settingsRepo.UpdateSessionRef(userID, session.SessionID, session.ID); err != nil {
		log.Printf("保存分类会话引用失败: %v", err)
	}
	return session, nil
}

func collectAssistantText(ch <-chan models.Message) string {
	var sb strings.Builder
	for msg := range ch {
		if msg.Role == models.MessageRoleAssistant && msg.Kind == models.MessageKindAgentMessageChunk {
			sb.WriteString(msg.Content)
		}
	}
	return strings.TrimSpace(sb.String())
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

func tagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func buildClassifyPrompt(template, content string, existingTags []string) string {
	tagsStr := "无"
	if len(existingTags) > 0 {
		tagsStr = strings.Join(existingTags, ", ")
	}
	p := strings.ReplaceAll(template, "{{content}}", content)
	return strings.ReplaceAll(p, "{{existing_tags}}", tagsStr)
}

func parseClassifyTags(text string) []string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			text = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	var arr []string
	if err := json.Unmarshal([]byte(text), &arr); err == nil {
		return normalizeTags(arr)
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &arr); err == nil {
			return normalizeTags(arr)
		}
	}
	return nil
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		t = strings.TrimPrefix(t, "#")
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func mergeTags(manual, auto []string) []string {
	if len(auto) == 0 {
		return manual
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(manual)+len(auto))
	for _, t := range manual {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, t := range auto {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
