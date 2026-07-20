package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// 迁移来源：
//   - legacyMain: ~/.nextAgent（改名前的主数据目录，含 db/session/config/acp-debug/launcher.log）
//   - legacyBins: ~/.nexusagent（更早的不一致二进制缓存目录，仅含 binaries/）
//
// 迁移目标：~/.openNexus（统一数据目录，见 config.go 的 resolveDataPaths）。
//
// 幂等：已迁移过则无操作；目标已存在且非空时跳过主目录迁移（目标优先）；失败仅记录警告不阻断启动。

// legacyDirNames 集中历史目录名，便于后续扩展。
const (
	legacyMainDir = ".nextAgent"
	legacyBinsDir = ".nexusagent"
	targetDir     = ".openNexus"
	legacyDBName  = "nexus.db"
	targetDBName  = "opennexus.db"
	binariesSubdir = "binaries"
)

// resolveDirs 基于 home 计算三类路径。抽出便于测试注入，避免依赖真实 home。
func resolveDirs(home string) (legacyMain, legacyBins, target string) {
	legacyMain = filepath.Join(home, legacyMainDir)
	legacyBins = filepath.Join(home, legacyBinsDir)
	target = filepath.Join(home, targetDir)
	return
}

// MigrateLegacyDataDir 在服务启动早期把改名前的历史数据目录迁移到 ~/.openNexus。
//
// 返回迁移条目数（用于日志）与错误。错误仅作信号，调用方应忽略后继续启动
// （与项目现有 RecoverActiveSessions/RestoreBinarySymlinks 风格一致）。
//
// 跳过开关：SKIP_DATA_MIGRATION=1 时立即返回（Docker / CI 场景显式禁用）。
func MigrateLegacyDataDir() (int, error) {
	if os.Getenv("SKIP_DATA_MIGRATION") == "1" {
		slog.Info("SKIP_DATA_MIGRATION=1，跳过历史数据目录迁移")
		return 0, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("获取用户主目录: %w", err)
	}
	return migrateDataDirs(home)
}

// migrateDataDirs 是 MigrateLegacyDataDir 的可测试核心：传入 home 即可运行全部逻辑。
func migrateDataDirs(home string) (int, error) {
	legacyMain, legacyBins, target := resolveDirs(home)
	moved := 0

	// 1. 主数据目录迁移（~/.nextAgent → ~/.openNexus）
	migrated, err := migrateMainDir(legacyMain, target)
	if err != nil {
		return moved, err
	}
	moved += migrated

	// 2. 文件级改名：target/nexus.db → target/opennexus.db（目标优先）
	if renamed, err := renameDBIfNeeded(target); err != nil {
		// db 改名失败不阻断（可能被占用），仅警告
		slog.Warn("数据库文件改名失败（不影响启动）", "err", err, "dir", target)
	} else {
		moved += renamed
	}

	// 3. 二进制缓存合并：~/.nexusagent/binaries/* → ~/.openNexus/binaries/
	if merged, err := mergeBinaryCache(legacyBins, target); err != nil {
		slog.Warn("二进制缓存迁移失败（不影响启动）", "err", err, "src", legacyBins)
	} else {
		moved += merged
	}

	return moved, nil
}

// migrateMainDir 处理主数据目录迁移。策略：目标优先。
//   - target 不存在且 legacyMain 存在 → os.Rename 整目录
//   - target 已存在且非空 → 跳过（仅日志提示），保留 legacyMain 原样
//   - 两者都不存在 → 无操作
func migrateMainDir(legacyMain, target string) (int, error) {
	legacyExists, err := dirExists(legacyMain)
	if err != nil {
		return 0, fmt.Errorf("检查历史目录 %s: %w", legacyMain, err)
	}
	if !legacyExists {
		return 0, nil
	}

	targetExists, targetEmpty, err := checkDir(target)
	if err != nil {
		return 0, fmt.Errorf("检查目标目录 %s: %w", target, err)
	}

	if targetExists && !targetEmpty {
		// 目标已有数据：保留目标，不迁移主目录（仍会单独合并 binaries）。
		slog.Info("目标数据目录已存在且非空，跳过主目录迁移；如需合并请手动处理",
			"target", target, "legacy", legacyMain)
		return 0, nil
	}

	// target 不存在或为空 → 整目录 rename（同分区原子操作）。
	if err := os.Rename(legacyMain, target); err != nil {
		return 0, fmt.Errorf("迁移主目录 %s → %s: %w", legacyMain, target, err)
	}
	slog.Info("已迁移主数据目录", "from", legacyMain, "to", target)
	return 1, nil
}

// renameDBIfNeeded 把 target/nexus.db 改名为 target/opennexus.db。
// 目标优先：若 opennexus.db 已存在则保留它，删除旧的 nexus.db。
// 两者都不存在则无操作。
func renameDBIfNeeded(target string) (int, error) {
	legacyDB := filepath.Join(target, legacyDBName)
	targetDB := filepath.Join(target, targetDBName)

	_, legacyErr := os.Stat(legacyDB)
	if os.IsNotExist(legacyErr) {
		return 0, nil // 无旧 db，无需改名
	}
	if legacyErr != nil {
		return 0, fmt.Errorf("检查旧数据库 %s: %w", legacyDB, legacyErr)
	}

	if _, err := os.Stat(targetDB); err == nil {
		// 新 db 已存在：删除旧 db（目标优先，避免遗留孤儿文件）
		if err := os.Remove(legacyDB); err != nil {
			return 0, fmt.Errorf("删除旧数据库 %s: %w", legacyDB, err)
		}
		slog.Info("目标数据库已存在，删除旧数据库", "removed", legacyDB, "kept", targetDB)
		return 1, nil
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("检查目标数据库 %s: %w", targetDB, err)
	}

	// 新 db 不存在：rename 旧 db
	if err := os.Rename(legacyDB, targetDB); err != nil {
		return 0, fmt.Errorf("改名数据库 %s → %s: %w", legacyDB, targetDB, err)
	}
	slog.Info("已迁移数据库文件", "from", legacyDB, "to", targetDB)
	return 1, nil
}

// mergeBinaryCache 把 legacyBins/binaries/* 逐条目移入 target/binaries/。
// 目标已有的条目跳过（目标优先）；迁移完成后清理空的 legacyBins 目录。
// 同时迁移 versions.json（位于 legacyBins 根，不在 binaries 子目录）。
func mergeBinaryCache(legacyBins, target string) (int, error) {
	legacyExists, err := dirExists(legacyBins)
	if err != nil {
		return 0, fmt.Errorf("检查旧二进制缓存目录 %s: %w", legacyBins, err)
	}
	if !legacyExists {
		return 0, nil
	}

	moved := 0
	targetBins := filepath.Join(target, binariesSubdir)

	// 迁移 versions.json（位于 legacyBins 根目录）
	legacyVer := filepath.Join(legacyBins, "versions.json")
	targetVer := filepath.Join(targetBins, "versions.json")
	if m, err := moveFileIfAbsent(legacyVer, targetVer); err != nil {
		slog.Warn("迁移 versions.json 失败（继续）", "err", err)
	} else {
		moved += m
	}

	// 迁移 binaries/ 子目录
	legacyBinDir := filepath.Join(legacyBins, binariesSubdir)
	if binDirExists, err := dirExists(legacyBinDir); err != nil {
		return moved, fmt.Errorf("检查旧 binaries 目录: %w", err)
	} else if binDirExists {
		entries, err := os.ReadDir(legacyBinDir)
		if err != nil {
			return moved, fmt.Errorf("读取旧 binaries 目录: %w", err)
		}
		for _, entry := range entries {
			src := filepath.Join(legacyBinDir, entry.Name())
			dst := filepath.Join(targetBins, entry.Name())
			if m, err := moveFileIfAbsent(src, dst); err != nil {
				slog.Warn("迁移二进制缓存条目失败（跳过）", "entry", entry.Name(), "err", err)
			} else {
				moved += m
			}
		}
	}

	// 清理空的 legacyBins（包含残留文件时不删，避免误删用户数据）。
	// 先尝试删除现已清空的 binaries 子目录，再判断 legacyBins 是否整体为空。
	_ = os.Remove(legacyBinDir) // 仅当空目录时成功；非空则保留
	if leftover, _ := os.ReadDir(legacyBins); len(leftover) == 0 {
		if err := os.Remove(legacyBins); err != nil {
			slog.Warn("清理旧二进制缓存目录失败（忽略）", "dir", legacyBins, "err", err)
		}
	} else {
		slog.Info("旧二进制缓存目录仍有残留文件，保留原样", "dir", legacyBins, "leftover", len(leftover))
	}

	return moved, nil
}

// moveFileIfAbsent 把 src 移到 dst，仅当 dst 不存在时。返回 1 表示移动，0 表示跳过。
func moveFileIfAbsent(src, dst string) (int, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("检查源 %s: %w", src, err)
	}
	if _, err := os.Stat(dst); err == nil {
		return 0, nil // 目标已存在，跳过（目标优先）
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("检查目标 %s: %w", dst, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, fmt.Errorf("创建目标父目录 %s: %w", filepath.Dir(dst), err)
	}
	if err := os.Rename(src, dst); err != nil {
		return 0, fmt.Errorf("移动 %s → %s: %w", src, dst, err)
	}
	return 1, nil
}

// dirExists 判断路径存在且是目录。
func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s 存在但不是目录", path)
	}
	return true, nil
}

// checkDir 返回 (exists, isEmpty)。不存在时 (false, true)。
func checkDir(path string) (exists, isEmpty bool, err error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	if !info.IsDir() {
		return false, false, fmt.Errorf("%s 存在但不是目录", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return true, false, fmt.Errorf("读取目录 %s: %w", path, err)
	}
	return true, len(entries) == 0, nil
}
