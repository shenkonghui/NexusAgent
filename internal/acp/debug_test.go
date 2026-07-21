package acp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"opennexus/internal/models"
)

func TestACPDebugger_BindPending_RoutesSessionNew(t *testing.T) {
	dir := t.TempDir()
	dbg := NewACPDebugger(DebugConfig{Enabled: true, Dir: dir})

	dbg.BindPending("cursor", "42")
	raw := `{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp","additionalDirectories":[],"mcpServers":[]}}`
	dbg.RouteLine("send", "cursor", raw)
	dbg.ClearPending("cursor")

	path := filepath.Join(dir, "42", "raw.ndjson")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("期望写入 42/raw.ndjson: %v", err)
	}
	var rec RawRecord
	if err := json.Unmarshal(data[:len(data)-1], &rec); err != nil {
		t.Fatalf("解析 raw 失败: %v", err)
	}
	if rec.DBSessionID != "42" {
		t.Errorf("DBSessionID = %q, 期望 42", rec.DBSessionID)
	}
	if rec.Direction != "send" {
		t.Errorf("Direction = %q", rec.Direction)
	}
}

func TestACPDebugger_NoPending_FallsBackToAgentFile(t *testing.T) {
	dir := t.TempDir()
	dbg := NewACPDebugger(DebugConfig{Enabled: true, Dir: dir})

	raw := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`
	dbg.RouteLine("send", "cursor", raw)

	path := filepath.Join(dir, "_cursor_.ndjson")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("期望写入 _cursor_.ndjson: %v", err)
	}
}

func TestACPDebugger_RegisterSession_RoutesByAgentID(t *testing.T) {
	dir := t.TempDir()
	dbg := NewACPDebugger(DebugConfig{Enabled: true, Dir: dir})
	dbg.RegisterSession("acp-abc", "7")

	raw := `{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":"acp-abc","prompt":[{"type":"text","text":"hi"}]}}`
	dbg.RouteLine("send", "cursor", raw)

	path := filepath.Join(dir, "7", "raw.ndjson")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("期望写入 7/raw.ndjson: %v", err)
	}
}

func TestACPDebugger_CleanupSession_RemovesDir(t *testing.T) {
	dir := t.TempDir()
	dbg := NewACPDebugger(DebugConfig{Enabled: true, Dir: dir})
	dbg.RegisterSession("acp-1", "9")
	dbg.RouteLine("send", "cursor", `{"jsonrpc":"2.0","method":"session/prompt","params":{"sessionId":"acp-1","prompt":[]}}`)
	if _, err := os.Stat(filepath.Join(dir, "9", "raw.ndjson")); err != nil {
		t.Fatalf("期望先写入 raw: %v", err)
	}

	dbg.CleanupSession("9")
	if _, err := os.Stat(filepath.Join(dir, "9")); !os.IsNotExist(err) {
		t.Fatal("期望 CleanupSession 删除 9/ 目录")
	}
}

func TestACPDebugger_RawRetentionMax100(t *testing.T) {
	dir := t.TempDir()
	dbg := NewACPDebugger(DebugConfig{Enabled: true, Dir: dir})
	dbg.RegisterSession("acp-x", "1")
	for i := 0; i < maxRawRecords+20; i++ {
		line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/prompt","params":{"sessionId":"acp-x","prompt":[]}}`, i)
		dbg.RouteLine("send", "cursor", line)
	}
	meta := dbg.Meta("1")
	if meta.RawCount != maxRawRecords {
		t.Fatalf("RawCount = %d, 期望 %d", meta.RawCount, maxRawRecords)
	}
	recs, err := dbg.ReadRaw("1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != maxRawRecords {
		t.Fatalf("ReadRaw len = %d, 期望 %d", len(recs), maxRawRecords)
	}
	// 应保留最新：最后一条 id 为 maxRawRecords+19
	var last struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(recs[len(recs)-1].Line, &last); err != nil {
		t.Fatal(err)
	}
	if last.ID != maxRawRecords+19 {
		t.Errorf("最后一条 id = %d, 期望 %d", last.ID, maxRawRecords+19)
	}
}

func TestAgentSessionID(t *testing.T) {
	if got := agentSessionID(&models.Session{SessionID: "stable", AgentSessionID: "acp-1"}); got != "acp-1" {
		t.Errorf("got %q, 期望 acp-1", got)
	}
	if got := agentSessionID(&models.Session{SessionID: "legacy-acp"}); got != "legacy-acp" {
		t.Errorf("got %q, 期望 legacy-acp", got)
	}
}
