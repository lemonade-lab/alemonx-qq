package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// pluginStoreDir selects the host-provided persistent store. Existing
// installations are copied once from their legacy user-config location when
// the store is still empty; the legacy copy is deliberately retained as a
// rollback path.
func pluginStoreDir(legacy string) (string, error) {
	target := strings.TrimSpace(os.Getenv("ALX_PLUGIN_STORE"))
	if target == "" {
		return legacy, nil
	}
	target = filepath.Clean(target)
	if filepath.Clean(legacy) == target {
		return target, nil
	}
	if err := migrateLegacyStore(legacy, target); err != nil {
		return "", err
	}
	return target, nil
}

func migrateLegacyStore(legacy, target string) error {
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	info, err := os.Stat(legacy)
	if os.IsNotExist(err) {
		return os.MkdirAll(target, 0o700)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("旧插件数据目录不是目录：%s", legacy)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".alx-plugin-store-migrate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyStoreTree(legacy, stage); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(stage, target)
}

func copyStoreTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := destination
		if relative != "." {
			target = filepath.Join(destination, relative)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			value, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			return os.Symlink(value, target)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("旧插件数据包含不受支持的文件：%s", path)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return closeErr
	})
}
