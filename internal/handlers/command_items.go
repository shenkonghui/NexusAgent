package handlers

import (
	acpsdk "github.com/coder/acp-go-sdk"

	acplocal "nexusagent/internal/acp"
)

// commandItem 是对外暴露的 slash command 描述。
type commandItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HasInput    bool   `json:"has_input"`
	Path        string `json:"path,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Kind        string `json:"kind"` // "command" | "agent"
}

func buildCommandItems(agentCmds []acpsdk.AvailableCommand, configured []acplocal.SlashCommand) []commandItem {
	cfgByName := make(map[string]acplocal.SlashCommand, len(configured))
	for _, c := range configured {
		cfgByName[c.Name] = c
	}
	items := make([]commandItem, 0, len(agentCmds))
	for _, cmd := range agentCmds {
		item := commandItem{
			Name:        cmd.Name,
			Description: cmd.Description,
			HasInput:    cmd.Input != nil,
			Kind:        "agent",
		}
		if cfg, ok := cfgByName[cmd.Name]; ok {
			item.Kind = "command"
			item.Path = cfg.Path
			item.Scope = cfg.Scope
			if item.Description == "" {
				item.Description = cfg.Description
			}
		}
		items = append(items, item)
	}
	return items
}
