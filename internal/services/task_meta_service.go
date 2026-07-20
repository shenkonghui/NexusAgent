package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"opennexus/internal/models"
	"opennexus/internal/repository"
)

// DefaultTaskTagPrompt 是任务自动打标签的默认提示词模板。
const DefaultTaskTagPrompt = `你是一个任务分类助手。根据用户任务描述，从预定义标签中选择最匹配的标签（1-3个）。只能从预定义标签中选择，不要创造新标签。
预定义标签：{{tags}}
任务描述：
{{prompt}}
仅输出 JSON 数组，例如 ["后端","mysql"]，不要输出其他任何文字。`

// DefaultTaskTitlePrompt 是任务自动生成标题的默认提示词模板。
const DefaultTaskTitlePrompt = `请为以下任务生成一个简短的标题（不超过15个字，不要加引号或书名号，概括任务核心意图）。
任务描述：{{prompt}}
仅输出标题文字，不要输出其他内容。`

// DefaultPredefinedTags 是首次使用时的默认预定义标签。
var DefaultPredefinedTags = []string{"后端", "前端", "部署", "数据库", "mysql", "文档", "测试", "调研"}

// TaskMetaExecutor 提供一次性 AI prompt 调用（临时会话，不落库）。
type TaskMetaExecutor interface {
	RunPromptOnce(ctx context.Context, agentType, modelValue, prompt string) (string, error)
}

// TaskMetaService 为任务自动打标签和生成标题。
type TaskMetaService struct {
	settingsRepo *repository.TaskSettingsRepository
	sessionRepo  *repository.SessionRepository
	executor     TaskMetaExecutor
}

func NewTaskMetaService(
	settingsRepo *repository.TaskSettingsRepository,
	sessionRepo *repository.SessionRepository,
	executor TaskMetaExecutor,
) *TaskMetaService {
	return &TaskMetaService{settingsRepo: settingsRepo, sessionRepo: sessionRepo, executor: executor}
}

// ProcessTask 为指定会话打标签和生成标题。应在后台 goroutine 中调用（fire-and-forget）。
// 任何失败均静默处理（仅记日志），不影响主对话流。
func (s *TaskMetaService) ProcessTask(userID, dbSessionID uint, prompt string) {
	settings, err := s.settingsRepo.FindByUserID(userID)
	if err != nil {
		slog.Warn("任务元数据：读取设置失败", "user", userID, "err", err)
		return
	}
	// 未配置 agent 则跳过
	agentType := strings.TrimSpace(settings.AgentType)
	if agentType == "" {
		return
	}
	modelValue := strings.TrimSpace(settings.ModelValue)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if settings.AutoTagEnabled {
		s.classifyTags(ctx, dbSessionID, prompt, settings, agentType, modelValue)
	}
	if settings.AutoTitleEnabled {
		s.generateTitle(ctx, dbSessionID, prompt, settings, agentType, modelValue)
	}
}

// classifyTags 调用 AI 对任务打标签并写入 Session.Tags。
func (s *TaskMetaService) classifyTags(ctx context.Context, dbSessionID uint, prompt string, settings *models.TaskSettings, agentType, modelValue string) {
	predefined := parseTagsJSON(settings.Tags)
	if len(predefined) == 0 {
		predefined = DefaultPredefinedTags
	}
	tmpl := settings.TagPrompt
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultTaskTagPrompt
	}
	built := buildTaskTagPrompt(tmpl, prompt, predefined)
	resp, err := s.executor.RunPromptOnce(ctx, agentType, modelValue, built)
	if err != nil {
		slog.Warn("任务元数据：分类调用失败", "session", dbSessionID, "err", err)
		return
	}
	tags := filterToPredefined(parseTaskTags(resp), predefined)
	if len(tags) == 0 {
		return
	}
	tagBytes, _ := json.Marshal(tags)
	if err := s.sessionRepo.UpdateTags(dbSessionID, string(tagBytes)); err != nil {
		slog.Warn("任务元数据：写入标签失败", "session", dbSessionID, "err", err)
	}
}

// generateTitle 调用 AI 为任务生成标题并写入 Session.Title。
func (s *TaskMetaService) generateTitle(ctx context.Context, dbSessionID uint, prompt string, settings *models.TaskSettings, agentType, modelValue string) {
	tmpl := settings.TitlePrompt
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultTaskTitlePrompt
	}
	built := strings.ReplaceAll(tmpl, "{{prompt}}", prompt)
	resp, err := s.executor.RunPromptOnce(ctx, agentType, modelValue, built)
	if err != nil {
		slog.Warn("任务元数据：标题调用失败", "session", dbSessionID, "err", err)
		return
	}
	title := cleanTaskTitle(resp)
	if title == "" {
		return
	}
	if err := s.sessionRepo.UpdateTitle(dbSessionID, title); err != nil {
		slog.Warn("任务元数据：写入标题失败", "session", dbSessionID, "err", err)
	}
}

func buildTaskTagPrompt(template, prompt string, tags []string) string {
	tagsStr := "无"
	if len(tags) > 0 {
		tagsStr = strings.Join(tags, ", ")
	}
	p := strings.ReplaceAll(template, "{{prompt}}", prompt)
	return strings.ReplaceAll(p, "{{tags}}", tagsStr)
}

// parseTaskTags 从 AI 文本响应中解析 JSON 标签数组。
func parseTaskTags(text string) []string {
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
		return dedupTags(arr)
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &arr); err == nil {
			return dedupTags(arr)
		}
	}
	return nil
}

// filterToPredefined 仅保留预定义标签中存在的项。
func filterToPredefined(candidates, predefined []string) []string {
	set := make(map[string]struct{}, len(predefined))
	for _, t := range predefined {
		set[strings.TrimSpace(t)] = struct{}{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, t := range candidates {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := set[t]; !ok {
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

func dedupTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
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

// parseTagsJSON 解析存储的标签 JSON 字符串。
func parseTagsJSON(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil
	}
	return arr
}

func cleanTaskTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`\"'“”‘’《》【】")
	s = strings.TrimSpace(s)
	// 截断到合理长度
	r := []rune(s)
	if len(r) > 30 {
		r = r[:30]
	}
	return string(r)
}
