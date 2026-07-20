package acp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"nexusagent/internal/models"
)

//go:embed registry.json
var embeddedRegistry []byte

// RegistryAgent 对应 ACP registry 中单个 agent 条目。
type RegistryAgent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Distribution Distribution `json:"distribution"`
}

// BinaryInstallInfo 记录二进制分发 agent 的安装信息，供延迟下载使用。
type BinaryInstallInfo struct {
	ArchiveURL string // 下载 URL
	Version    string // agent 版本
	BinaryCmd  string // 二进制相对路径（如 "./crow-cli"）
}

// BinaryRegistry 存储所有 binary 分发 agent 的安装信息（agentID → info）。
// 在 RegistryToAgentConfigs 中填充，在创建 Backend 时读取。
var BinaryRegistry = make(map[string]BinaryInstallInfo)

// NpxDist 描述 npx 分发的子结构。
type NpxDist struct {
	Package string            `json:"package"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// UvxDist 描述 uvx 分发的子结构。
type UvxDist struct {
	Package string            `json:"package"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// BinaryArchEntry 描述二进制分发中单个平台架构条目。
type BinaryArchEntry struct {
	Archive string            `json:"archive"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Distribution 描述 agent 的分发/启动方式。
// 支持 ACP registry 的嵌套结构：{"npx": {...}}、{"binary": {平台: {...}}}、{"uvx": {...}}。
type Distribution struct {
	Type    string            // "npx" | "binary" | "uvx"
	Package string            // npx/uvx 的包名
	Args    []string          // npx/uvx 的启动参数
	Env     map[string]string // npx/uvx 的环境变量

	// binary 分发字段
	BinaryCmd     string            // 二进制命令（相对路径，如 "./crow-cli"）
	BinaryArgs    []string          // 二进制参数
	BinaryEnv     map[string]string // 二进制环境变量
	BinaryArchive string            // 二进制下载 URL
}

// UnmarshalJSON 实现 Distribution 的自定义 JSON 解析，适配嵌套格式。
func (d *Distribution) UnmarshalJSON(data []byte) error {
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("解析 distribution: %w", err)
	}

	// 尝试解析 npx 类型
	if npxRaw, ok := raw["npx"]; ok {
		var npxDist NpxDist
		if err := json.Unmarshal(npxRaw, &npxDist); err != nil {
			return fmt.Errorf("解析 npx distribution: %w", err)
		}
		d.Type = "npx"
		d.Package = npxDist.Package
		d.Args = npxDist.Args
		d.Env = npxDist.Env
		return nil
	}

	// 尝试解析 uvx 类型
	if uvxRaw, ok := raw["uvx"]; ok {
		var uvxDist UvxDist
		if err := json.Unmarshal(uvxRaw, &uvxDist); err != nil {
			return fmt.Errorf("解析 uvx distribution: %w", err)
		}
		d.Type = "uvx"
		d.Package = uvxDist.Package
		d.Args = uvxDist.Args
		d.Env = uvxDist.Env
		return nil
	}

	// 尝试解析 binary 类型
	if binaryRaw, ok := raw["binary"]; ok {
		d.Type = "binary"
		// 选择当前平台对应的二进制配置
		entry, err := selectBinaryEntry(binaryRaw)
		if err != nil {
			return err
		}
		d.BinaryCmd = entry.Cmd
		d.BinaryArgs = entry.Args
		d.BinaryEnv = entry.Env
		d.BinaryArchive = entry.Archive
		return nil
	}

	// 未知分发类型，保留为空以便上层跳过
	return nil
}

// platformKey 返回当前平台的 registry 键名（如 "darwin-aarch64"）。
func platformKey() string {
	osMap := map[string]string{
		"darwin":  "darwin",
		"linux":   "linux",
		"windows": "windows",
	}
	archMap := map[string]string{
		"arm64": "aarch64",
		"amd64": "x86_64",
	}
	osKey, ok := osMap[runtime.GOOS]
	if !ok {
		osKey = runtime.GOOS
	}
	archKey, ok := archMap[runtime.GOARCH]
	if !ok {
		archKey = runtime.GOARCH
	}
	return osKey + "-" + archKey
}

// selectBinaryEntry 从 binary 分发中选取当前平台对应的条目。
func selectBinaryEntry(binaryRaw json.RawMessage) (*BinaryArchEntry, error) {
	platforms := make(map[string]BinaryArchEntry)
	if err := json.Unmarshal(binaryRaw, &platforms); err != nil {
		return nil, fmt.Errorf("解析 binary distribution: %w", err)
	}

	key := platformKey()
	if entry, ok := platforms[key]; ok {
		return &entry, nil
	}

	// 回退：尝试查找任意可用平台
	for _, entry := range platforms {
		slog.Warn("未找到当前平台的 binary 分发，使用回退平台", "current", key, "fallback", entry.Cmd)
		return &entry, nil
	}

	return nil, fmt.Errorf("binary 分发中没有可用的平台条目")
}

// registryJSON 是 ACP registry 的顶层结构。
type registryJSON struct {
	Version string          `json:"version"`
	Agents  []RegistryAgent `json:"agents"`
}

const registryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

// FetchEmbeddedRegistry 从内嵌的 registry.json 加载 agent 列表。
func FetchEmbeddedRegistry() ([]RegistryAgent, error) {
	var reg registryJSON
	if err := json.Unmarshal(embeddedRegistry, &reg); err != nil {
		return nil, fmt.Errorf("解析内嵌 registry: %w", err)
	}
	return reg.Agents, nil
}

// FindEmbeddedRegistryAgent 按 ID 在内嵌 registry 中查找单个 agent。
// 未找到返回 (nil, nil)。调用方通常接着用 (*RegistryAgent).ToAgentConfig() 取默认 command/args。
// 注意:对 binary 类 agent 调用 ToAgentConfig 会触发 buildCommand 的副作用——写入 BinaryRegistry，
// 这是 NewBackendFromAgentConfig 正确选择 BinaryBackend 的前提。
func FindEmbeddedRegistryAgent(agentID string) (*RegistryAgent, error) {
	agents, err := FetchEmbeddedRegistry()
	if err != nil {
		return nil, err
	}
	return findAgentInList(agents, agentID), nil
}

// FindOnlineRegistryAgent 从 CDN 最新 registry 中按 ID 查单个 agent。
// 用于"更新"按钮:获取该 agent 最新的 distribution(含 version/archiveURL)。
// 未找到返回 (nil, nil);网络或解析错误返回 error(调用方可回退到内嵌 registry)。
func FindOnlineRegistryAgent(agentID string) (*RegistryAgent, error) {
	doc, err := FetchRegistryFull()
	if err != nil {
		return nil, err
	}
	return findAgentInList(doc.Agents, agentID), nil
}

// findAgentInList 在 agent 列表中按 ID 查找，未找到返回 nil。
func findAgentInList(agents []RegistryAgent, agentID string) *RegistryAgent {
	for i := range agents {
		if agents[i].ID == agentID {
			return &agents[i]
		}
	}
	return nil
}

// FetchRegistry 从 ACP 官方注册表拉取 agent 列表（在线更新用）。
// 已废弃：仅返回 agents，丢失顶层 version。新代码请用 FetchRegistryFull。
func FetchRegistry() ([]RegistryAgent, error) {
	reg, err := FetchRegistryFull()
	if err != nil {
		return nil, err
	}
	return reg.Agents, nil
}

// FetchRegistryFull 从 ACP 官方注册表拉取完整 registry（含顶层 version）。
// 超时 15s；非 200 或 JSON 解析失败返回错误。
func FetchRegistryFull() (*RegistryDocument, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(registryURL)
	if err != nil {
		return nil, fmt.Errorf("请求 registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry 返回状态码 %d", resp.StatusCode)
	}

	var reg registryJSON
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("解析 registry JSON: %w", err)
	}
	return &RegistryDocument{Version: reg.Version, Agents: reg.Agents}, nil
}

// RegistryDocument 是在线拉取的完整 registry 文档（含顶层 version）。
type RegistryDocument struct {
	Version string
	Agents  []RegistryAgent
}

// ToAgentConfig 将 registry agent 转换为 models.AgentConfig。
func (ra *RegistryAgent) ToAgentConfig() (models.AgentConfig, error) {
	command, args, err := ra.buildCommand()
	if err != nil {
		return models.AgentConfig{}, err
	}

	argsJSON := ""
	if len(args) > 0 {
		b, _ := json.Marshal(args)
		argsJSON = string(b)
	}

	enabled := false
	return models.AgentConfig{
		Type:        ra.ID,
		DisplayName: ra.Name,
		Description: ra.Description,
		Command:     command,
		Args:        argsJSON,
		APIKeyEnv:   "", // 子进程自动继承父进程环境变量
		Timeout:     "300s",
		Enabled:     &enabled,
	}, nil
}

// buildCommand 根据 distribution 构建 command + args。
func (ra *RegistryAgent) buildCommand() (string, []string, error) {
	switch ra.Distribution.Type {
	case "npx":
		// 格式: npm exec --include=optional --yes <package> -- [args...]
		// --include=optional 确保安装 native binary 等可选依赖（npm 11.x 默认跳过）
		// 必须用 "--" 分隔，否则 package 参数（如 --acp）会被 npm 吞掉而非传给子进程
		args := []string{"exec", "--include=optional", "--yes", ra.Distribution.Package}
		if len(ra.Distribution.Args) > 0 {
			args = append(args, "--")
			args = append(args, ra.Distribution.Args...)
		}
		return "npm", args, nil
	case "uvx":
		// 格式: uvx <package> [args...]
		args := []string{ra.Distribution.Package}
		args = append(args, ra.Distribution.Args...)
		return "uvx", args, nil
	case "binary":
		// 不在此处下载——仅注册到 BinaryRegistry，待用户启用后由 BinaryBackend 延迟下载
		BinaryRegistry[ra.ID] = BinaryInstallInfo{
			ArchiveURL: ra.Distribution.BinaryArchive,
			Version:    ra.Version,
			BinaryCmd:  ra.Distribution.BinaryCmd,
		}
		// 返回相对路径作为占位 command，后续 BinaryBackend 会替换为完整路径
		args := ra.Distribution.BinaryArgs
		// cursor-agent 启动时会要求 Workspace Trust 交互确认，不传 --trust 会卡住无法完成 ACP 握手。
		// registry 默认 args 不含 --trust，这里补上（官方 ACP 文档推荐用法）。
		if isTrustRequiredAgent(ra.ID) && !containsArg(args, "--trust") {
			args = append(args, "--trust")
		}
		return ra.Distribution.BinaryCmd, args, nil
	default:
		return "", nil, fmt.Errorf("不支持的 distribution type: %s", ra.Distribution.Type)
	}
}

// trustRequiredAgents 列出启动时需要 --trust 参数才能跳过交互确认的 binary agent ID。
// 这些 agent 不传 --trust 会卡在 Workspace Trust 提示，无法完成 ACP 握手。
var trustRequiredAgents = map[string]bool{
	"cursor": true,
}

func isTrustRequiredAgent(agentID string) bool {
	return trustRequiredAgents[agentID]
}

// containsArgs 检查 args 中是否已包含指定参数。
func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

// RegistryToAgentConfigs 将 registry agent 列表转换为 AgentConfig 列表。
func RegistryToAgentConfigs(agents []RegistryAgent) []models.AgentConfig {
	var configs []models.AgentConfig
	for _, ra := range agents {
		cfg, err := ra.ToAgentConfig()
		if err != nil {
			slog.Warn("跳过不支持的 registry agent", "id", ra.ID, "err", err)
			continue
		}
		configs = append(configs, cfg)
	}
	return configs
}

// AgentConfigSyncer 暴露合并 registry agent 所需的最小持久化能力。
// *repository.AgentConfigRepository 天然满足该接口。
type AgentConfigSyncer interface {
	FindByType(agentType string) (*models.AgentConfig, error)
	Create(cfg *models.AgentConfig) error
	Update(cfg *models.AgentConfig) error
}

// SyncRegistryToStore 把 registry agents 合并进存储：
//   - 新 agent：以 enabled=false 创建（不自动注册，等待用户在设置中启用）
//   - 已有 agent：仅更新 DisplayName/Description，保留用户修改的 command/args/env/enabled
//
// 对正在运行的后端/registrar 零影响。返回 (新增数, 更新数)。
func SyncRegistryToStore(agents []RegistryAgent, store AgentConfigSyncer) (added, updated int, err error) {
	configs := RegistryToAgentConfigs(agents)
	for _, cfg := range configs {
		existing, _ := store.FindByType(cfg.Type)
		if existing != nil {
			existing.DisplayName = cfg.DisplayName
			existing.Description = cfg.Description
			if uerr := store.Update(existing); uerr != nil {
				slog.Warn("更新 registry agent 失败", "type", cfg.Type, "err", uerr)
				continue
			}
			updated++
		} else {
			enabled := false
			cfg.Enabled = &enabled
			if cerr := store.Create(&cfg); cerr != nil {
				slog.Warn("写入 registry agent 失败", "type", cfg.Type, "err", cerr)
				continue
			}
			added++
		}
	}
	slog.Info("registry agent 已同步到存储", "total", len(agents), "added", added, "updated", updated)
	return added, updated, nil
}
