package acp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxRawRecords 每个 raw 文件最多保留的原始报文条数（超出时丢弃最旧）。
const maxRawRecords = 100

// DebugConfig 是 ACPDebugger 的运行时配置（由 config.ACPDebugConfig 映射）。
type DebugConfig struct {
	Enabled bool
	Dir     string
}

// RawRecord 是 raw.ndjson 中的一行。
type RawRecord struct {
	TS          string          `json:"ts"`
	Direction   string          `json:"direction"` // send | recv
	SessionID   string          `json:"session_id,omitempty"`
	DBSessionID string          `json:"db_session_id,omitempty"`
	Line        json.RawMessage `json:"line"`
}

// EventRecord 是 events.ndjson 中的一行。
type EventRecord struct {
	TS          string          `json:"ts"`
	Event       string          `json:"event"`
	SessionID   string          `json:"session_id,omitempty"`
	DBSessionID string          `json:"db_session_id,omitempty"`
	Detail      json.RawMessage `json:"detail,omitempty"`
}

// DebugMeta 是 debug 元信息。
type DebugMeta struct {
	Enabled    bool   `json:"enabled"`
	Dir        string `json:"dir"`
	EventCount int    `json:"event_count"`
	RawCount   int    `json:"raw_count"`
	LastTS     string `json:"last_ts,omitempty"`
}

// ACPDebugger 按 session 路由并持久化 ACP 报文与高层事件。
// 所有 I/O 错误仅 slog.Warn，绝不影响主流程。
type ACPDebugger struct {
	cfg        DebugConfig
	mu         sync.Mutex
	sessions   map[string]string // acpSessionID → dbSessionID
	pending    map[string]string // agentType → dbSessionID（session/new 发出前暂存）
	writers    map[string]*bufio.Writer
	files      map[string]*os.File
	lineCounts map[string]int // 各文件当前行数（用于 raw 截断）
}

// NewACPDebugger 创建调试器；Enabled=false 时也可创建，写入会 no-op。
func NewACPDebugger(cfg DebugConfig) *ACPDebugger {
	return &ACPDebugger{
		cfg:        cfg,
		sessions:   make(map[string]string),
		pending:    make(map[string]string),
		writers:    make(map[string]*bufio.Writer),
		files:      make(map[string]*os.File),
		lineCounts: make(map[string]int),
	}
}

// Enabled 返回是否启用捕获。
func (d *ACPDebugger) Enabled() bool {
	return d != nil && d.cfg.Enabled
}

// Dir 返回数据目录。
func (d *ACPDebugger) Dir() string {
	if d == nil {
		return ""
	}
	return d.cfg.Dir
}

// RegisterSession 建立 acpSessionID → dbSessionID 映射。
func (d *ACPDebugger) RegisterSession(acpSessionID, dbSessionID string) {
	if d == nil || acpSessionID == "" || dbSessionID == "" {
		return
	}
	d.mu.Lock()
	d.sessions[acpSessionID] = dbSessionID
	d.mu.Unlock()
}

// Unregister 移除映射。
func (d *ACPDebugger) Unregister(acpSessionID string) {
	if d == nil || acpSessionID == "" {
		return
	}
	d.mu.Lock()
	delete(d.sessions, acpSessionID)
	d.mu.Unlock()
}

// CleanupSession 关闭该会话的文件句柄并删除 acp-debug/<dbSessionID>/ 目录。
func (d *ACPDebugger) CleanupSession(dbSessionID string) {
	if d == nil || dbSessionID == "" || d.cfg.Dir == "" {
		return
	}
	clean := filepath.Clean(dbSessionID)
	if clean == "." || clean == ".." || strings.ContainsRune(clean, os.PathSeparator) {
		return
	}
	dir := filepath.Join(d.cfg.Dir, clean)
	rel, err := filepath.Rel(d.cfg.Dir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return
	}
	prefix := dir + string(os.PathSeparator)
	d.mu.Lock()
	for path, w := range d.writers {
		if path == dir || strings.HasPrefix(path, prefix) {
			_ = w.Flush()
			delete(d.writers, path)
			delete(d.lineCounts, path)
		}
	}
	for path, f := range d.files {
		if path == dir || strings.HasPrefix(path, prefix) {
			_ = f.Close()
			delete(d.files, path)
			delete(d.lineCounts, path)
		}
	}
	for k, v := range d.sessions {
		if v == clean {
			delete(d.sessions, k)
		}
	}
	for k, v := range d.pending {
		if v == clean {
			delete(d.pending, k)
		}
	}
	d.mu.Unlock()
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("acp-debug 清理会话目录失败", "dir", dir, "err", err)
	}
}

// BindPending 在 session/new 发出前绑定 agentType → dbSessionID，使无 sessionId 的报文可落入会话目录。
func (d *ACPDebugger) BindPending(agentType, dbSessionID string) {
	if d == nil || agentType == "" || dbSessionID == "" {
		return
	}
	d.mu.Lock()
	d.pending[agentType] = dbSessionID
	d.mu.Unlock()
}

// ClearPending 清除 agentType 上的 pending 绑定。
func (d *ACPDebugger) ClearPending(agentType string) {
	if d == nil || agentType == "" {
		return
	}
	d.mu.Lock()
	delete(d.pending, agentType)
	d.mu.Unlock()
}

// RouteLine 解析一行 JSON-RPC，按 sessionId 路由到对应 session 的 raw.ndjson；
// 无法识别时若有 BindPending 则用 pending，否则回退到连接级 _<agentType>_.ndjson。
// 同步写入，避免与 BindPending/ClearPending 竞态。
func (d *ACPDebugger) RouteLine(direction, agentType, rawLine string) {
	if !d.Enabled() || rawLine == "" {
		return
	}
	d.routeLineSync(direction, agentType, rawLine)
}

func (d *ACPDebugger) routeLineSync(direction, agentType, rawLine string) {
	sessionID := extractSessionID(rawLine)
	d.mu.Lock()
	dbID := ""
	if sessionID != "" {
		dbID = d.sessions[sessionID]
	}
	if dbID == "" && agentType != "" {
		dbID = d.pending[agentType]
	}
	d.mu.Unlock()

	rec := RawRecord{
		TS:          time.Now().UTC().Format(time.RFC3339Nano),
		Direction:   direction,
		SessionID:   sessionID,
		DBSessionID: dbID,
		Line:        json.RawMessage(rawLine),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		slog.Warn("acp-debug 序列化 raw 失败", "err", err)
		return
	}

	var path string
	if dbID != "" {
		path = filepath.Join(d.cfg.Dir, dbID, "raw.ndjson")
	} else {
		name := agentType
		if name == "" {
			name = "unknown"
		}
		path = filepath.Join(d.cfg.Dir, "_"+name+"_.ndjson")
	}
	d.appendLine(path, data)
}

// LogEvent 记录高层语义事件到 events.ndjson。
func (d *ACPDebugger) LogEvent(dbSessionID, event, acpSessionID string, detail any) {
	if !d.Enabled() || dbSessionID == "" || event == "" {
		return
	}
	go d.logEventSync(dbSessionID, event, acpSessionID, detail)
}

func (d *ACPDebugger) logEventSync(dbSessionID, event, acpSessionID string, detail any) {
	var detailRaw json.RawMessage
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			slog.Warn("acp-debug 序列化 event detail 失败", "err", err)
		} else {
			detailRaw = b
		}
	}
	rec := EventRecord{
		TS:          time.Now().UTC().Format(time.RFC3339Nano),
		Event:       event,
		SessionID:   acpSessionID,
		DBSessionID: dbSessionID,
		Detail:      detailRaw,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		slog.Warn("acp-debug 序列化 event 失败", "err", err)
		return
	}
	path := filepath.Join(d.cfg.Dir, dbSessionID, "events.ndjson")
	d.appendLine(path, data)
}

// Meta 返回指定 dbSession 的调试元信息。
func (d *ACPDebugger) Meta(dbSessionID string) DebugMeta {
	meta := DebugMeta{Enabled: d.Enabled(), Dir: d.Dir()}
	if !d.Enabled() || dbSessionID == "" {
		return meta
	}
	rawPath := filepath.Join(d.cfg.Dir, dbSessionID, "raw.ndjson")
	evtPath := filepath.Join(d.cfg.Dir, dbSessionID, "events.ndjson")
	meta.RawCount = countNDJSONLines(rawPath)
	meta.EventCount = countNDJSONLines(evtPath)
	meta.LastTS = lastTS(rawPath, evtPath)
	return meta
}

// ReadRaw 分页读取 raw.ndjson（since 为行偏移，从 0 起）。
func (d *ACPDebugger) ReadRaw(dbSessionID string, since, limit int) ([]RawRecord, error) {
	if !d.Enabled() || dbSessionID == "" {
		return nil, nil
	}
	path := filepath.Join(d.cfg.Dir, dbSessionID, "raw.ndjson")
	lines, err := readNDJSONLines(path, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]RawRecord, 0, len(lines))
	for _, line := range lines {
		var rec RawRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// ReadEvents 分页读取 events.ndjson。
func (d *ACPDebugger) ReadEvents(dbSessionID string, since, limit int) ([]EventRecord, error) {
	if !d.Enabled() || dbSessionID == "" {
		return nil, nil
	}
	path := filepath.Join(d.cfg.Dir, dbSessionID, "events.ndjson")
	lines, err := readNDJSONLines(path, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]EventRecord, 0, len(lines))
	for _, line := range lines {
		var rec EventRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

func (d *ACPDebugger) appendLine(path string, data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	w, err := d.writerLocked(path)
	if err != nil {
		slog.Warn("acp-debug 打开文件失败", "path", path, "err", err)
		return
	}
	if _, err := w.Write(data); err != nil {
		slog.Warn("acp-debug 写入失败", "path", path, "err", err)
		return
	}
	if err := w.WriteByte('\n'); err != nil {
		slog.Warn("acp-debug 写入换行失败", "path", path, "err", err)
		return
	}
	if err := w.Flush(); err != nil {
		slog.Warn("acp-debug flush 失败", "path", path, "err", err)
	}
	d.lineCounts[path]++
	if isRawDebugPath(path) && d.lineCounts[path] > maxRawRecords {
		d.trimRawLocked(path)
	}
}

func (d *ACPDebugger) writerLocked(path string) (*bufio.Writer, error) {
	if w, ok := d.writers[path]; ok {
		return w, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(f, 64*1024)
	d.files[path] = f
	d.writers[path] = w
	if _, ok := d.lineCounts[path]; !ok {
		d.lineCounts[path] = countNDJSONLines(path)
	}
	return w, nil
}

func isRawDebugPath(path string) bool {
	base := filepath.Base(path)
	return base == "raw.ndjson" || (strings.HasPrefix(base, "_") && strings.HasSuffix(base, "_.ndjson"))
}

// trimRawLocked 关闭句柄后重写文件，仅保留最近 maxRawRecords 行。调用方须已持锁。
func (d *ACPDebugger) trimRawLocked(path string) {
	if w := d.writers[path]; w != nil {
		_ = w.Flush()
		delete(d.writers, path)
	}
	if f := d.files[path]; f != nil {
		_ = f.Close()
		delete(d.files, path)
	}
	lines, err := readNDJSONLines(path, 0, 0)
	if err != nil || len(lines) <= maxRawRecords {
		d.lineCounts[path] = len(lines)
		return
	}
	keep := lines[len(lines)-maxRawRecords:]
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		slog.Warn("acp-debug 截断创建临时文件失败", "path", path, "err", err)
		return
	}
	for _, line := range keep {
		if _, err := out.Write(line); err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			slog.Warn("acp-debug 截断写入失败", "path", path, "err", err)
			return
		}
		if _, err := out.Write([]byte{'\n'}); err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			slog.Warn("acp-debug 截断写换行失败", "path", path, "err", err)
			return
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("acp-debug 截断替换失败", "path", path, "err", err)
		return
	}
	d.lineCounts[path] = maxRawRecords
}

func extractSessionID(rawLine string) string {
	var envelope struct {
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(rawLine), &envelope); err != nil {
		return ""
	}
	if id := sessionIDFromJSON(envelope.Params); id != "" {
		return id
	}
	return sessionIDFromJSON(envelope.Result)
}

func sessionIDFromJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m["sessionId"]
	if !ok {
		return ""
	}
	var id string
	if err := json.Unmarshal(v, &id); err != nil {
		return ""
	}
	return id
}

func countNDJSONLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	n := 0
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) > 0 {
			n++
		}
	}
	return n
}

func lastTS(paths ...string) string {
	var latest string
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
		var last string
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			var rec struct {
				TS string `json:"ts"`
			}
			if json.Unmarshal(line, &rec) == nil && rec.TS != "" {
				last = rec.TS
			}
		}
		f.Close()
		if last > latest {
			latest = last
		}
	}
	return latest
}

func readNDJSONLines(path string, since, limit int) ([][]byte, error) {
	if since < 0 {
		since = 0
	}
	// limit<=0 表示不限制条数
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	idx := 0
	var out [][]byte
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if idx < since {
			idx++
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		out = append(out, cp)
		idx++
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("读取 ndjson: %w", err)
	}
	return out, nil
}

// --- tee 包装器：按行捕获 stdin/stdout，透传原始字节给 SDK ---

type teeReader struct {
	r         io.Reader
	dbg       *ACPDebugger
	direction string
	agentType string
	buf       []byte
}

func (t *teeReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 && t.dbg != nil && t.dbg.Enabled() {
		t.scan(p[:n])
	}
	return n, err
}

func (t *teeReader) scan(data []byte) {
	t.buf = append(t.buf, data...)
	for {
		i := bytes.IndexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimSpace(t.buf[:i]))
		t.buf = append([]byte(nil), t.buf[i+1:]...)
		if line != "" {
			t.dbg.RouteLine(t.direction, t.agentType, line)
		}
	}
}

type teeWriter struct {
	w         io.WriteCloser
	dbg       *ACPDebugger
	direction string
	agentType string
	buf       []byte
}

func (t *teeWriter) Write(p []byte) (int, error) {
	if t.dbg != nil && t.dbg.Enabled() {
		t.scan(p)
	}
	return t.w.Write(p)
}

func (t *teeWriter) Close() error {
	return t.w.Close()
}

func (t *teeWriter) scan(data []byte) {
	t.buf = append(t.buf, data...)
	for {
		i := bytes.IndexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimSpace(t.buf[:i]))
		t.buf = append([]byte(nil), t.buf[i+1:]...)
		if line != "" {
			t.dbg.RouteLine(t.direction, t.agentType, line)
		}
	}
}
