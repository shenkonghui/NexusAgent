package acp

import (
	"encoding/json"
	"testing"

	"opennexus/internal/models"
)

// fakeRegistryStore 是 AgentConfigSyncer 的内存实现，用于单测 SyncRegistryToStore。
type fakeRegistryStore struct {
	byType map[string]*models.AgentConfig
	// 调用计数（便于断言 Create/Update 是否被调用）
	creates int
	updates int
}

func newFakeRegistryStore(seeds ...*models.AgentConfig) *fakeRegistryStore {
	s := &fakeRegistryStore{byType: map[string]*models.AgentConfig{}}
	for _, c := range seeds {
		cc := *c
		s.byType[cc.Type] = &cc
	}
	return s
}

func (s *fakeRegistryStore) FindByType(agentType string) (*models.AgentConfig, error) {
	if c, ok := s.byType[agentType]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (s *fakeRegistryStore) Create(cfg *models.AgentConfig) error {
	s.creates++
	cp := *cfg
	s.byType[cp.Type] = &cp
	return nil
}

func (s *fakeRegistryStore) Update(cfg *models.AgentConfig) error {
	s.updates++
	cp := *cfg
	s.byType[cp.Type] = &cp
	return nil
}

// npxRegistryAgent 构造一个 npx 分发的 registry agent（最常见类型，可被 ToAgentConfig 处理）。
func npxRegistryAgent(id, name, desc string) RegistryAgent {
	return RegistryAgent{
		ID:          id,
		Name:        name,
		Version:     "1.0.0",
		Description: desc,
		Distribution: Distribution{
			Type:    "npx",
			Package: "@acp/" + id,
		},
	}
}

func TestSyncRegistryToStore_AddsNewAgentsAsDisabled(t *testing.T) {
	store := newFakeRegistryStore()
	agents := []RegistryAgent{
		npxRegistryAgent("foo", "Foo", "foo desc"),
		npxRegistryAgent("bar", "Bar", "bar desc"),
	}

	added, updated, err := SyncRegistryToStore(agents, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}
	if store.creates != 2 {
		t.Fatalf("store.creates = %d, want 2", store.creates)
	}

	// 新 agent 必须以 enabled=false 入库
	for _, id := range []string{"foo", "bar"} {
		got := store.byType[id]
		if got == nil {
			t.Fatalf("agent %s not stored", id)
		}
		if got.Enabled == nil || *got.Enabled {
			t.Fatalf("agent %s enabled = %v, want false", id, got.Enabled)
		}
		if got.DisplayName == "" {
			t.Fatalf("agent %s DisplayName empty", id)
		}
	}
}

func TestSyncRegistryToStore_UpdatesExistingPreservesUserEdits(t *testing.T) {
	// 预置一个已启用的 agent，带用户自定义 command/args/env —— 这些字段在同步后必须保留。
	enabled := true
	existing := &models.AgentConfig{
		ID:          1,
		Type:        "foo",
		DisplayName: "Old Name",
		Description: "old desc",
		Command:     "my-custom-cmd",
		Args:        `["--x"]`,
		Env:         `{"KEY":"val"}`,
		Enabled:     &enabled,
	}
	store := newFakeRegistryStore(existing)

	agents := []RegistryAgent{
		// 名称/描述已变化
		npxRegistryAgent("foo", "New Name", "new desc"),
		// 一个全新的 agent
		npxRegistryAgent("baz", "Baz", "baz desc"),
	}

	added, updated, err := SyncRegistryToStore(agents, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 1 || updated != 1 {
		t.Fatalf("added=%d updated=%d, want added=1 updated=1", added, updated)
	}

	got := store.byType["foo"]
	if got.DisplayName != "New Name" {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, "New Name")
	}
	if got.Description != "new desc" {
		t.Fatalf("Description = %q, want %q", got.Description, "new desc")
	}
	// 用户自定义字段必须保留
	if got.Command != "my-custom-cmd" {
		t.Fatalf("Command = %q, want %q (user edit should be preserved)", got.Command, "my-custom-cmd")
	}
	if got.Args != `["--x"]` {
		t.Fatalf("Args = %q, want %q (user edit should be preserved)", got.Args, `["--x"]`)
	}
	if got.Env != `{"KEY":"val"}` {
		t.Fatalf("Env = %q, want %q (user edit should be preserved)", got.Env, `{"KEY":"val"}`)
	}
	if got.Enabled == nil || !*got.Enabled {
		t.Fatalf("Enabled = %v, want true (user enabled state should be preserved)", got.Enabled)
	}

	// 新 agent 仍是禁用
	baz := store.byType["baz"]
	if baz == nil || baz.Enabled == nil || *baz.Enabled {
		t.Fatalf("new agent baz should be stored disabled")
	}
}

func TestSyncRegistryToStore_EmptyInput(t *testing.T) {
	store := newFakeRegistryStore()
	added, updated, err := SyncRegistryToStore(nil, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 0 || updated != 0 {
		t.Fatalf("added=%d updated=%d, want 0/0", added, updated)
	}
}

func TestSyncRegistryToStore_SkipsUnsupportedDistribution(t *testing.T) {
	// 未知 distribution 类型会被 RegistryToAgentConfigs 跳过（buildCommand 报错），不计入 added/updated。
	store := newFakeRegistryStore()
	agents := []RegistryAgent{
		{ID: "bad", Name: "Bad", Distribution: Distribution{Type: "unknown"}},
		npxRegistryAgent("good", "Good", "good"),
	}
	added, updated, err := SyncRegistryToStore(agents, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1 (unsupported agent should be skipped)", added)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}
	if _, ok := store.byType["bad"]; ok {
		t.Fatalf("unsupported agent 'bad' should not be stored")
	}
}

func TestFindEmbeddedRegistryAgent_Found(t *testing.T) {
	// claude-acp 是内嵌 registry 中真实存在的 npx 类 agent
	ra, err := FindEmbeddedRegistryAgent("claude-acp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ra == nil {
		t.Fatal("expected to find claude-acp in embedded registry, got nil")
	}
	if ra.ID != "claude-acp" {
		t.Fatalf("ID = %q, want %q", ra.ID, "claude-acp")
	}
	if ra.Name == "" {
		t.Fatal("Name should not be empty")
	}
	// 确认能转出可用的默认配置（npx 类 → command 为 npm）
	cfg, err := ra.ToAgentConfig()
	if err != nil {
		t.Fatalf("ToAgentConfig failed: %v", err)
	}
	if cfg.Command != "npm" {
		t.Errorf("command = %q, want %q (npx distribution)", cfg.Command, "npm")
	}
}

func TestFindEmbeddedRegistryAgent_NotFound(t *testing.T) {
	ra, err := FindEmbeddedRegistryAgent("does-not-exist-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ra != nil {
		t.Fatalf("expected nil for unknown ID, got %+v", ra)
	}
}

// TestBuildCommand_CursorBinaryAddsTrust 验证 cursor 这种 binary agent 的 args 自动补 --trust。
// cursor-agent 不传 --trust 会卡在 Workspace Trust 交互，无法完成 ACP 握手。
func TestBuildCommand_CursorBinaryAddsTrust(t *testing.T) {
	ra := &RegistryAgent{
		ID:      "cursor",
		Name:     "Cursor",
		Version:  "2026.06.26",
		Distribution: Distribution{
			Type:         "binary",
			BinaryCmd:    "./dist-package/cursor-agent",
			BinaryArgs:   []string{"acp"}, // registry 默认不含 --trust
			BinaryArchive: "https://example.com/cursor.tar.gz",
		},
	}
	cfg, err := ra.ToAgentConfig()
	if err != nil {
		t.Fatalf("ToAgentConfig failed: %v", err)
	}
	var args []string
	_ = json.Unmarshal([]byte(cfg.Args), &args)
	found := false
	for _, a := range args {
		if a == "--trust" {
			found = true
		}
	}
	if !found {
		t.Errorf("cursor binary args = %v, 期望自动补 --trust", args)
	}
}

// TestBuildCommand_NonTrustAgentNoTrust 验证非 trust-required 的 binary agent 不被强加 --trust。
func TestBuildCommand_NonTrustAgentNoTrust(t *testing.T) {
	ra := &RegistryAgent{
		ID:      "amp-acp",
		Name:     "Amp",
		Version:  "1.0.0",
		Distribution: Distribution{
			Type:         "binary",
			BinaryCmd:    "./amp",
			BinaryArgs:   []string{"acp"},
			BinaryArchive: "https://example.com/amp.tar.gz",
		},
	}
	cfg, err := ra.ToAgentConfig()
	if err != nil {
		t.Fatalf("ToAgentConfig failed: %v", err)
	}
	var args []string
	_ = json.Unmarshal([]byte(cfg.Args), &args)
	for _, a := range args {
		if a == "--trust" {
			t.Errorf("amp-acp 不该被强加 --trust，args = %v", args)
		}
	}
}
