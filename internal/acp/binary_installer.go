package acp

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const binaryDownloadRetries = 3

// binariesCacheDir 是二进制分发 agent 的下载缓存根目录。
func binariesCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录: %w", err)
	}
	return filepath.Join(home, ".nexusagent", "binaries"), nil
}

// ensureBinaryInstalled 确保指定 agent 的二进制已下载解压到缓存目录。
// 返回二进制文件的完整路径。已缓存则跳过下载。
func ensureBinaryInstalled(agentID, version, archiveURL, cmd string) (string, error) {
	cacheRoot, err := binariesCacheDir()
	if err != nil {
		return "", err
	}
	// 缓存目录：<cacheRoot>/<agentID>-<version>/
	cacheDir := filepath.Join(cacheRoot, agentID+"-"+version)

	// 已缓存则直接返回（支持压缩包内嵌子目录）
	if cached, err := resolveBinaryPath(cacheDir, cmd); err == nil {
		slog.Info("binary 已缓存", "agent", agentID, "path", cached)
		return cached, nil
	}

	slog.Info("开始下载 binary", "agent", agentID, "url", archiveURL)
	if err := downloadAndExtract(archiveURL, cacheDir); err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", fmt.Errorf("下载解压 binary: %w", err)
	}

	binaryPath, err := resolveBinaryPath(cacheDir, cmd)
	if err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", err
	}

	// 确保二进制可执行
	_ = os.Chmod(binaryPath, 0o755)

	slog.Info("binary 下载解压完成", "agent", agentID, "path", binaryPath)
	return binaryPath, nil
}

// downloadAndExtract 下载 archiveURL 并解压到 destDir。
func downloadAndExtract(archiveURL, destDir string) error {
	// 创建目标目录
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("创建缓存目录: %w", err)
	}

	// 下载
	tmpFile, err := downloadArchive(archiveURL)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	// 根据后缀解压
	lower := strings.ToLower(archiveURL)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(tmpFile, destDir)
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return extractTarBz2(tmpFile, destDir)
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(tmpFile, destDir)
	case strings.HasSuffix(lower, ".tar"):
		return extractTar(tmpFile, destDir)
	default:
		return fmt.Errorf("不支持的压缩格式: %s", archiveURL)
	}
}

// downloadArchive 下载文件到临时文件，返回临时文件路径。
func downloadArchive(url string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	var lastErr error
	for attempt := 1; attempt <= binaryDownloadRetries; attempt++ {
		if attempt > 1 {
			wait := time.Duration(attempt) * 2 * time.Second
			slog.Info("重试下载 binary", "url", url, "attempt", attempt, "wait", wait)
			time.Sleep(wait)
		}
		tmpFile, err := downloadArchiveOnce(client, url)
		if err == nil {
			return tmpFile, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func downloadArchiveOnce(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("请求 %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 %s 返回状态码 %d", url, resp.StatusCode)
	}

	f, err := os.CreateTemp("", "nexus-binary-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("下载写入: %w", err)
	}
	f.Close()
	return f.Name(), nil
}

// resolveBinaryPath 在缓存目录中定位二进制文件，支持压缩包内嵌子目录。
func resolveBinaryPath(cacheDir, cmd string) (string, error) {
	binaryPath := filepath.Join(cacheDir, filepath.Clean(strings.TrimPrefix(cmd, "./")))
	if _, err := os.Stat(binaryPath); err == nil {
		return binaryPath, nil
	}

	base := filepath.Base(filepath.Clean(strings.TrimPrefix(cmd, "./")))
	var found string
	_ = filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) == base {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("解压后未找到二进制文件: %s", binaryPath)
	}
	return found, nil
}

// extractTarGz 解压 .tar.gz 到 destDir。
func extractTarGz(filePath, destDir string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip 解压: %w", err)
	}
	defer gz.Close()

	return untar(gz, destDir)
}

// extractTarBz2 解压 .tar.bz2 到 destDir。
func extractTarBz2(filePath, destDir string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return untar(bzip2.NewReader(f), destDir)
}

// extractTar 解压 .tar 到 destDir。
func extractTar(filePath, destDir string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return untar(f, destDir)
}

// untar 解压 tar 到 destDir。
func untar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 条目: %w", err)
		}

		target := filepath.Join(destDir, filepath.Clean(header.Name))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			_ = os.Symlink(header.Linkname, target)
		}
	}
	return nil
}

// extractZip 解压 .zip 到 destDir。
func extractZip(filePath, destDir string) error {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.Clean(f.Name))

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		src, err := f.Open()
		if err != nil {
			return err
		}

		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			src.Close()
			return err
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
