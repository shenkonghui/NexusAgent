package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile 写入一个标记文件，便于迁移后校验内容是否保留。
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(b)
}

// assertExists 断言路径存在。
func assertExists(t *testing.T, path, label string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s 应存在，实际: %v (path=%s)", label, err, path)
	}
}

// assertNotExists 断言路径不存在。
func assertNotExists(t *testing.T, path, label string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s 不应存在，实际 err=%v (path=%s)", label, err, path)
	}
}

// TestMigrate_NoLegacyDirs 场景1：无任何旧目录 → 无副作用，moved=0。
func TestMigrate_NoLegacyDirs(t *testing.T) {
	home := t.TempDir()
	moved, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("migrateDataDirs 错误: %v", err)
	}
	if moved != 0 {
		t.Errorf("无旧目录时 moved = %d，期望 0", moved)
	}
	_, _, target := resolveDirs(home)
	assertNotExists(t, target, "目标目录")
}

// TestMigrate_OnlyNextAgent 场景2：仅 ~/.nextAgent（含 nexus.db）→ 整目录迁移 + db 改名。
func TestMigrate_OnlyNextAgent(t *testing.T) {
	home := t.TempDir()
	legacyMain, _, target := resolveDirs(home)

	// 构造历史数据：nexus.db + session/ + config.yaml
	writeTestFile(t, filepath.Join(legacyMain, legacyDBName), "db-content")
	writeTestFile(t, filepath.Join(legacyMain, "session", "ws-1", "README"), "ws")
	writeTestFile(t, filepath.Join(legacyMain, "config.yaml"), "port: 8080")

	moved, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("migrateDataDirs 错误: %v", err)
	}
	if moved < 2 {
		t.Errorf("moved = %d，期望 >= 2（主目录 + db 改名）", moved)
	}

	// 旧目录应消失（整目录 rename）
	assertNotExists(t, legacyMain, "旧主目录")
	// 新目录应有所有内容
	assertExists(t, filepath.Join(target, targetDBName), "新 db")
	assertExists(t, filepath.Join(target, "session", "ws-1", "README"), "迁移的 session")
	assertExists(t, filepath.Join(target, "config.yaml"), "迁移的 config")
	// 旧 db 名不应残留
	assertNotExists(t, filepath.Join(target, legacyDBName), "旧 db 名")

	// 内容应保留
	if got := readFile(t, filepath.Join(target, targetDBName)); got != "db-content" {
		t.Errorf("db 内容丢失: got %q", got)
	}
}

// TestMigrate_OnlyNexusAgent 场景3：仅 ~/.nexusagent → binaries 合并到 target/binaries，旧目录清理。
func TestMigrate_OnlyNexusAgent(t *testing.T) {
	home := t.TempDir()
	_, legacyBins, target := resolveDirs(home)

	// 构造历史二进制缓存
	writeTestFile(t, filepath.Join(legacyBins, "versions.json"), `{"version":"1"}`)
	writeTestFile(t, filepath.Join(legacyBins, "binaries", "agent-a", "run"), "agent-a-bin")
	writeTestFile(t, filepath.Join(legacyBins, "binaries", "agent-b", "run"), "agent-b-bin")

	moved, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("migrateDataDirs 错误: %v", err)
	}
	if moved < 3 {
		t.Errorf("moved = %d，期望 >= 3（versions.json + 2 个 agent）", moved)
	}

	// 旧目录应被清理
	assertNotExists(t, legacyBins, "旧二进制缓存目录")
	// 新位置应有所有内容
	assertExists(t, filepath.Join(targetBins(target), "versions.json"), "迁移的 versions.json")
	assertExists(t, filepath.Join(targetBins(target), "agent-a", "run"), "迁移的 agent-a")
	assertExists(t, filepath.Join(targetBins(target), "agent-b", "run"), "迁移的 agent-b")

	// 内容保留
	if got := readFile(t, filepath.Join(targetBins(target), "versions.json")); got != `{"version":"1"}` {
		t.Errorf("versions.json 内容丢失: got %q", got)
	}
}

func targetBins(target string) string {
	return filepath.Join(target, binariesSubdir)
}

// TestMigrate_TargetExistsNonEmpty 场景4：target 已存在且非空 → 跳过主目录迁移，仍合并 binaries。
func TestMigrate_TargetExistsNonEmpty(t *testing.T) {
	home := t.TempDir()
	legacyMain, legacyBins, target := resolveDirs(home)

	// 历史主目录（含旧 db）
	writeTestFile(t, filepath.Join(legacyMain, legacyDBName), "old-db")
	// 历史二进制缓存
	writeTestFile(t, filepath.Join(legacyBins, "binaries", "agent-a", "run"), "agent-a-bin")
	// 目标已存在且非空（新版跑过一次，已有 opennexus.db）
	writeTestFile(t, filepath.Join(target, targetDBName), "new-db")

	moved, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("migrateDataDirs 错误: %v", err)
	}
	if moved == 0 {
		t.Error("期望至少迁移 binaries（moved > 0），实际 0")
	}

	// 主目录保留原样（未迁移）
	assertExists(t, legacyMain, "旧主目录（应保留）")
	assertExists(t, filepath.Join(legacyMain, legacyDBName), "旧主目录的 nexus.db（应保留）")
	// 目标的 db 不被覆盖
	if got := readFile(t, filepath.Join(target, targetDBName)); got != "new-db" {
		t.Errorf("目标 db 被覆盖: got %q，期望 new-db", got)
	}
	// binaries 仍应合并进 target
	assertExists(t, filepath.Join(targetBins(target), "agent-a", "run"), "合并的 agent-a")
}

// TestMigrate_BothDBsExist 场景5：nexus.db 与 opennexus.db 同时存在 → 保留 opennexus.db，删 nexus.db。
func TestMigrate_BothDBsExist(t *testing.T) {
	home := t.TempDir()
	legacyMain, _, target := resolveDirs(home)

	// 目标已存在：含 opennexus.db + nexus.db（冲突场景）
	writeTestFile(t, filepath.Join(target, targetDBName), "new-db")
	writeTestFile(t, filepath.Join(target, legacyDBName), "old-db")
	// 历史主目录也存在（触发冲突跳过路径，但 db 改名逻辑仍跑）
	writeTestFile(t, filepath.Join(legacyMain, "leftover"), "x")

	moved, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("migrateDataDirs 错误: %v", err)
	}

	// 新 db 保留，旧 db 删除
	assertExists(t, filepath.Join(target, targetDBName), "新 db")
	assertNotExists(t, filepath.Join(target, legacyDBName), "旧 db（应删除）")
	if got := readFile(t, filepath.Join(target, targetDBName)); got != "new-db" {
		t.Errorf("新 db 内容被覆盖: got %q", got)
	}
	_ = moved // 计数不强制校验，关键是行为正确
}

// TestMigrate_SkipEnv 场景6：SKIP_DATA_MIGRATION=1 → 立即返回 0，不读盘。
func TestMigrate_SkipEnv(t *testing.T) {
	home := t.TempDir()
	legacyMain, _, _ := resolveDirs(home)
	// 造一个旧目录，验证开关生效时不被迁移
	writeTestFile(t, filepath.Join(legacyMain, legacyDBName), "db")

	t.Setenv("SKIP_DATA_MIGRATION", "1")
	moved, err := MigrateLegacyDataDir()
	if err != nil {
		t.Fatalf("MigrateLegacyDataDir 错误: %v", err)
	}
	if moved != 0 {
		t.Errorf("SKIP_DATA_MIGRATION=1 时 moved = %d，期望 0", moved)
	}
	// 旧目录应原样保留
	assertExists(t, legacyMain, "旧主目录（开关启用时应保留）")
}

// TestMigrate_Idempotent 幂等性：连续运行两次，第二次无副作用。
func TestMigrate_Idempotent(t *testing.T) {
	home := t.TempDir()
	legacyMain, _, target := resolveDirs(home)
	writeTestFile(t, filepath.Join(legacyMain, legacyDBName), "db-content")

	// 第一次迁移
	moved1, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("第一次迁移错误: %v", err)
	}
	if moved1 == 0 {
		t.Fatal("第一次迁移应有动作")
	}
	// 第二次运行：旧目录已不在，应无操作
	moved2, err := migrateDataDirs(home)
	if err != nil {
		t.Fatalf("第二次迁移错误: %v", err)
	}
	if moved2 != 0 {
		t.Errorf("第二次迁移 moved = %d，期望 0（幂等）", moved2)
	}
	// 数据仍完整
	if got := readFile(t, filepath.Join(target, targetDBName)); got != "db-content" {
		t.Errorf("二次迁移后 db 内容丢失: got %q", got)
	}
}

// TestResolveDirs 验证路径计算正确性。
func TestResolveDirs(t *testing.T) {
	legacyMain, legacyBins, target := resolveDirs("/fake/home")
	if legacyMain != filepath.Join("/fake/home", legacyMainDir) {
		t.Errorf("legacyMain = %q", legacyMain)
	}
	if legacyBins != filepath.Join("/fake/home", legacyBinsDir) {
		t.Errorf("legacyBins = %q", legacyBins)
	}
	if target != filepath.Join("/fake/home", targetDir) {
		t.Errorf("target = %q", target)
	}
}
