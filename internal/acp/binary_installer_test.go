package acp

import (
	"archive/tar"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoveBinaryCache 验证 RemoveBinaryCache 只删除指定 agent 的版本目录，不影响其它 agent。
// 使用唯一测试前缀避免污染真实缓存；测试结束时无条件清理残留。
func TestRemoveBinaryCache(t *testing.T) {
	const targetAgent = "__test_removecache_target__"
	const otherAgent = "__test_removecache_other__"
	cleanup := func() {
		root, _ := binariesCacheDir()
		if root == "" {
			return
		}
		pat := filepath.Join(root, "__test_removecache_*")
		for _, m := range mustGlob(pat) {
			_ = os.RemoveAll(m)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	root, err := binariesCacheDir()
	if err != nil {
		t.Fatalf("binariesCacheDir 失败: %v", err)
	}
	// 造目标 agent 的多个版本目录 + 另一个 agent 的目录
	mkDir(t, filepath.Join(root, targetAgent+"-1.0.0"))
	mkDir(t, filepath.Join(root, targetAgent+"-2.0.0"))
	mkDir(t, filepath.Join(root, otherAgent+"-1.0.0"))

	removed, err := RemoveBinaryCache(targetAgent)
	if err != nil {
		t.Fatalf("RemoveBinaryCache 失败: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2（目标 agent 有两个版本目录）", removed)
	}

	// 目标 agent 的目录应全部删除
	if exists(filepath.Join(root, targetAgent+"-1.0.0")) {
		t.Error("target 1.0.0 目录未被删除")
	}
	if exists(filepath.Join(root, targetAgent+"-2.0.0")) {
		t.Error("target 2.0.0 目录未被删除")
	}
	// 其它 agent 不受影响
	if !exists(filepath.Join(root, otherAgent+"-1.0.0")) {
		t.Error("other agent 目录被误删")
	}
}

// TestRemoveBinaryCache_NoCache 验证缓存目录不存在时不报错、返回 0。
func TestRemoveBinaryCache_NoCache(t *testing.T) {
	const ghost = "__test_removecache_ghost__"
	cleanup := func() {
		root, _ := binariesCacheDir()
		for _, m := range mustGlob(filepath.Join(root, ghost+"-*")) {
			_ = os.RemoveAll(m)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	removed, err := RemoveBinaryCache(ghost)
	if err != nil {
		t.Fatalf("无缓存时应返回 nil 错误，got: %v", err)
	}
	if removed != 0 {
		t.Fatalf("无缓存时 removed = %d, want 0", removed)
	}
}

func mkDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("创建测试目录 %s 失败: %v", path, err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mustGlob(pattern string) []string {
	matches, _ := filepath.Glob(pattern)
	return matches
}

// TestSwitchSymlink_CreatesAndSwitches 验证 symlink 创建与切换。
func TestSwitchSymlink_CreatesAndSwitches(t *testing.T) {
	const agent = "__test_symlink_switch__"
	cleanup := func() {
		root, _ := binariesCacheDir()
		for _, m := range mustGlob(filepath.Join(root, agent+"*")) {
			_ = os.RemoveAll(m)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	root, _ := binariesCacheDir()
	link := filepath.Join(root, agent)
	dir1 := filepath.Join(root, agent+"-1.0.0")
	dir2 := filepath.Join(root, agent+"-2.0.0")
	mkDir(t, dir1)
	mkDir(t, dir2)

	// 首次创建 symlink → 指向 1.0.0
	if err := switchSymlink(link, dir1); err != nil {
		t.Fatalf("首次 switchSymlink 失败: %v", err)
	}
	if !exists(filepath.Join(link, ".")) { // 通过 link 访问目录
		t.Error("symlink 创建后无法通过 link 访问")
	}

	// 切换到 2.0.0
	if err := switchSymlink(link, dir2); err != nil {
		t.Fatalf("切换 switchSymlink 失败: %v", err)
	}
	target, _ := os.Readlink(link)
	if !strings.HasSuffix(target, agent+"-2.0.0") {
		t.Errorf("切换后 symlink 指向 %q, 期望指向 2.0.0", target)
	}
}

// TestSaveLoadVersions_RoundTrip 验证 versions.json 写入再读出一致。
func TestSaveLoadVersions_RoundTrip(t *testing.T) {
	const agent = "__test_versions_io__"
	cleanup := func() {
		root, _ := binariesCacheDir()
		_ = os.Remove(filepath.Join(root, "versions.json"))
	}
	cleanup()
	t.Cleanup(cleanup)

	records := map[string]VersionRecord{
		agent: {Version: "1.2.3", ArchiveURL: "https://example.com/a.tar.gz", UpdatedAt: "2026-07-20T00:00:00Z"},
	}
	if err := saveVersions(records); err != nil {
		t.Fatalf("saveVersions 失败: %v", err)
	}
	loaded, err := loadVersions()
	if err != nil {
		t.Fatalf("loadVersions 失败: %v", err)
	}
	if loaded[agent].Version != "1.2.3" {
		t.Errorf("读回 version = %q, want 1.2.3", loaded[agent].Version)
	}
	if loaded[agent].ArchiveURL != "https://example.com/a.tar.gz" {
		t.Errorf("读回 archive_url 不匹配: %q", loaded[agent].ArchiveURL)
	}
}

// TestRestoreBinarySymlinks_RebuildsAfterRestart 模拟重启场景：
// versions.json 记录了 2.0.0，但 symlink 不存在（模拟重启后内存丢失）→ 调用后应重建指向 2.0.0。
func TestRestoreBinarySymlinks_RebuildsAfterRestart(t *testing.T) {
	const agent = "__test_symlink_restore__"
	cleanup := func() {
		root, _ := binariesCacheDir()
		for _, m := range mustGlob(filepath.Join(root, agent+"*")) {
			_ = os.RemoveAll(m)
		}
		// 清理 versions.json 中该 agent 的记录
		if recs, err := loadVersions(); err == nil {
			delete(recs, agent)
			_ = saveVersions(recs)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	root, _ := binariesCacheDir()
	dir2 := filepath.Join(root, agent+"-2.0.0")
	mkDir(t, dir2)

	// 写 versions.json 记录 2.0.0（模拟之前更新过）
	recs := map[string]VersionRecord{
		agent: {Version: "2.0.0", ArchiveURL: "https://example.com/2.tar.gz", UpdatedAt: "2026-07-20T00:00:00Z"},
	}
	if err := saveVersions(recs); err != nil {
		t.Fatalf("saveVersions 失败: %v", err)
	}
	// symlink 不存在（模拟重启后状态）
	link := filepath.Join(root, agent)
	if exists(link) {
		t.Fatal("前置条件失败：symlink 不应存在")
	}

	// 调用恢复
	restored, err := RestoreBinarySymlinks()
	if err != nil {
		t.Fatalf("RestoreBinarySymlinks 失败: %v", err)
	}
	if restored < 1 {
		t.Fatalf("restored = %d, 期望 >= 1", restored)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("恢复后 symlink 不存在: %v", err)
	}
	if !strings.HasSuffix(target, agent+"-2.0.0") {
		t.Errorf("恢复后 symlink 指向 %q, 期望 2.0.0", target)
	}
}

// TestEnsureBinaryDownloaded_FailureCleansUp 验证下载失败时不留半成品目录。
// 这是 update-from-registry 的关键防护：避免空目录导致后续 Prepare 无限重试下载失败的版本。
func TestEnsureBinaryDownloaded_FailureCleansUp(t *testing.T) {
	const agent = "__test_ensure_fail__"
	cleanup := func() {
		root, _ := binariesCacheDir()
		for _, m := range mustGlob(filepath.Join(root, agent+"-*")) {
			_ = os.RemoveAll(m)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	// 指向一个必定失败的 URL（不存在的 host）
	err := EnsureBinaryDownloaded(agent, "1.0.0", "http://127.0.0.1:1/nonexistent.tar.gz", "./fake-bin")
	if err == nil {
		t.Fatal("期望下载失败返回 error，got nil")
	}

	root, _ := binariesCacheDir()
	// 半成品目录应被清理
	if exists(filepath.Join(root, agent+"-1.0.0.downloading")) {
		t.Error("下载失败的半成品目录 .downloading 未被清理")
	}
	if exists(filepath.Join(root, agent+"-1.0.0")) {
		t.Error("下载失败时不应创建目标目录")
	}
}

// TestEnsureBinaryDownloaded_SuccessReplacesCache 验证成功下载时目标目录就位。
func TestEnsureBinaryDownloaded_SuccessReplacesCache(t *testing.T) {
	const agent = "__test_ensure_ok__"
	const version = "2.0.0"
	cleanup := func() {
		root, _ := binariesCacheDir()
		for _, m := range mustGlob(filepath.Join(root, agent+"-*")) {
			_ = os.RemoveAll(m)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	// 用 httptest 提供一个含 fake-bin 的 tar.gz
	url := serveFakeBinaryArchive(t, "fake-bin")
	if err := EnsureBinaryDownloaded(agent, version, url, "./fake-bin"); err != nil {
		t.Fatalf("期望下载成功，got error: %v", err)
	}
	root, _ := binariesCacheDir()
	target := filepath.Join(root, agent+"-"+version)
	if !exists(target) {
		t.Fatalf("目标目录 %s 不存在", target)
	}
	if !exists(filepath.Join(target, "fake-bin")) {
		t.Error("目标目录中缺少 fake-bin")
	}
	// 临时目录应已被 rename 掉
	if exists(filepath.Join(root, agent+"-"+version+".downloading")) {
		t.Error("成功后临时目录 .downloading 应已被 rename 掉")
	}
}

// serveFakeBinaryArchive 启动一个 httptest server，提供包含 name 文件的 tar.gz，返回其 URL。
func serveFakeBinaryArchive(t *testing.T, name string) string {
	t.Helper()
	// 构造 tar.gz 到临时文件
	tmp, err := os.CreateTemp("", "fake-archive-*.tar.gz")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmp.Name()) })
	gw := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: 3}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("写 tar header 失败: %v", err)
	}
	if _, err := tw.Write([]byte("hi\n")); err != nil {
		t.Fatalf("写 tar 内容失败: %v", err)
	}
	tw.Close()
	gw.Close()
	tmp.Close()

	// 用 server 提供该文件
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, tmp.Name())
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/archive.tar.gz"
}
