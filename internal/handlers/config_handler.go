package handlers

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// AgentsConfigView 是前端可查看/编辑的 agents 配置视图（skills/commands/rules 的目录路径配置）。
type AgentsConfigView struct {
	Skills   DirConfigView `json:"skills"`
	Commands DirConfigView `json:"commands"`
	Rules    DirConfigView `json:"rules"`
}

// DirConfigView 是单个 dirs 配置的视图。
type DirConfigView struct {
	UserDirs    []string `json:"user_dirs"`
	ProjectDirs []string `json:"project_dirs"`
}

// ConfigHandler 提供 config.yaml 中 agents 相关配置的读取与更新。
type ConfigHandler struct {
	configPath string
}

// NewConfigHandler 创建 ConfigHandler。
func NewConfigHandler(configPath string) *ConfigHandler {
	return &ConfigHandler{configPath: configPath}
}

// GetAgentsConfig GET /api/v1/config/agents
// 返回 config.yaml 中 agents 块下的 skills、commands、rules user_dirs/project_dirs 配置。
func (h *ConfigHandler) GetAgentsConfig(c *gin.Context) {
	cfg, err := readConfigRaw(h.configPath)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "CONFIG_READ_ERROR", "读取配置文件失败")
		return
	}
	view := extractAgentsView(cfg)
	Success(c, http.StatusOK, view)
}

// UpdateAgentsConfig PUT /api/v1/config/agents
// 更新 config.yaml 中 agents 块下的 skills、commands、rules user_dirs/project_dirs 配置。
// 注意：修改后需要重启服务才能生效。
func (h *ConfigHandler) UpdateAgentsConfig(c *gin.Context) {
	var view AgentsConfigView
	if err := c.ShouldBindJSON(&view); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_JSON", "请求参数格式错误")
		return
	}

	// 读取现有配置
	root, err := readConfigRaw(h.configPath)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "CONFIG_READ_ERROR", "读取配置文件失败")
		return
	}

	// 更新 agents 部分
	updateAgentsNode(root, &view)

	// 写回文件
	if err := writeConfigRaw(h.configPath, root); err != nil {
		Fail(c, http.StatusInternalServerError, "CONFIG_WRITE_ERROR", "写入配置文件失败")
		return
	}

	Success(c, http.StatusOK, gin.H{"message": "配置已更新，重启服务后生效"})
}

// readConfigRaw 读取并解析 config.yaml 为 yaml.Node 树，保留注释和格式。
func readConfigRaw(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return &root, nil
}

// writeConfigRaw 将 yaml.Node 树写回文件。
func writeConfigRaw(path string, root *yaml.Node) error {
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// extractAgentsView 从 yaml.Node 中提取 agents 配置视图。
func extractAgentsView(root *yaml.Node) AgentsConfigView {
	view := AgentsConfigView{}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return view
	}
	mapping := root.Content[0] // 根映射节点
	agentsNode := findMappingValue(mapping, "agents")
	if agentsNode == nil {
		return view
	}

	// skills
	if skillsNode := findMappingValue(agentsNode, "skills"); skillsNode != nil {
		view.Skills = extractDirConfig(skillsNode)
	}
	// commands
	if cmdsNode := findMappingValue(agentsNode, "commands"); cmdsNode != nil {
		view.Commands = extractDirConfig(cmdsNode)
	}
	// rules
	if rulesNode := findMappingValue(agentsNode, "rules"); rulesNode != nil {
		view.Rules = extractDirConfig(rulesNode)
	}
	return view
}

// extractDirConfig 从 yaml.Node 中提取 user_dirs / project_dirs。
func extractDirConfig(node *yaml.Node) DirConfigView {
	dc := DirConfigView{}
	if node.Kind != yaml.MappingNode {
		return dc
	}
	if userNode := findMappingValue(node, "user_dirs"); userNode != nil {
		dc.UserDirs = extractStringList(userNode)
	}
	if projNode := findMappingValue(node, "project_dirs"); projNode != nil {
		dc.ProjectDirs = extractStringList(projNode)
	}
	return dc
}

// extractStringList 从序列节点提取字符串列表。
func extractStringList(node *yaml.Node) []string {
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	result := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			result = append(result, item.Value)
		}
	}
	return result
}

// updateAgentsNode 更新 agents 节点下的 skills/commands/rules user_dirs/project_dirs。
func updateAgentsNode(root *yaml.Node, view *AgentsConfigView) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return
	}
	mapping := root.Content[0]
	agentsNode := findMappingValue(mapping, "agents")
	if agentsNode == nil {
		return
	}

	// 更新 skills
	if skillsNode := findMappingValue(agentsNode, "skills"); skillsNode != nil {
		updateDirConfigNode(skillsNode, &view.Skills)
	}
	// 更新 commands
	if cmdsNode := findMappingValue(agentsNode, "commands"); cmdsNode != nil {
		updateDirConfigNode(cmdsNode, &view.Commands)
	}
	// 更新 rules
	if rulesNode := findMappingValue(agentsNode, "rules"); rulesNode != nil {
		updateDirConfigNode(rulesNode, &view.Rules)
	}
}

// updateDirConfigNode 更新单个 dir 配置节点的 user_dirs / project_dirs。
func updateDirConfigNode(node *yaml.Node, dc *DirConfigView) {
	if node.Kind != yaml.MappingNode {
		return
	}
	if dc.UserDirs != nil {
		setSequenceValue(node, "user_dirs", dc.UserDirs)
	}
	if dc.ProjectDirs != nil {
		setSequenceValue(node, "project_dirs", dc.ProjectDirs)
	}
}

// setSequenceValue 设置映射节点中指定键的序列值。
func setSequenceValue(mapping *yaml.Node, key string, values []string) {
	if mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Value == key {
			// 替换值节点
			seqNode := &yaml.Node{
				Kind:    yaml.SequenceNode,
				Tag:     "!!seq",
				Content: make([]*yaml.Node, len(values)),
			}
			for j, v := range values {
				seqNode.Content[j] = &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!!str",
					Value: v,
				}
			}
			mapping.Content[i+1] = seqNode
			return
		}
	}
}

// findMappingValue 在映射节点中查找指定键对应的值节点。
func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
