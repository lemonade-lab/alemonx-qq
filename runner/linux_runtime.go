package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/lemonade-lab/alemonx-qq/internal/qqruntime"
	"github.com/ulikunitz/xz"
)

const (
	linuxShellAsset = "NapCat.Shell.zip"
)

type linuxQQAsset struct {
	Name   string
	URL    string
	Kind   string
	SHA256 string
}

// linuxQQReleaseAssets are official Tencent Linux QQ packages. The values are
// deliberately host-owned: no browser, manifest or downloaded installer can
// replace them with an arbitrary URL.
// linuxQQReleaseAssetForEnvironment selects a package format without making
// package-manager availability a user-visible prerequisite. A managed runtime
// can unpack the reviewed DEB payload even on musl or an unknown distribution;
// its bundled loader and libraries are what make that payload runnable.
func linuxQQReleaseAssetForEnvironment(environment string) (linuxQQAsset, error) {
	packageManager := ""
	if _, err := exec.LookPath("apt-get"); err == nil {
		packageManager = "apt"
	} else if _, err := exec.LookPath("dnf"); err == nil {
		packageManager = "dnf"
	}
	if packageManager == "" && environment == "managed-runtime" {
		packageManager = "apt"
	}
	return linuxQQReleaseAssetFor(runtime.GOARCH, packageManager)
}

// linuxQQReleaseAssetFor is kept free of host inspection so every supported
// package can be tested. Tencent does not publish a stable checksum manifest
// for these direct downloads; the reviewed hashes below are therefore part of
// this release contract and must be updated together with linuxQQVersion.
func linuxQQReleaseAssetFor(goarch, packageManager string) (linuxQQAsset, error) {
	asset, err := qqruntime.AssetFor(goarch, packageManager)
	if err != nil {
		if packageManager != "apt" && packageManager != "dnf" {
			return linuxQQAsset{}, errors.New("未检测到 APT 或 DNF，无法选择 Linux QQ 官方安装包")
		}
		return linuxQQAsset{}, fmt.Errorf("Linux %s 暂不支持自动安装 QQ 运行时", goarch)
	}
	return linuxQQAsset{Name: asset.Name, Kind: asset.Kind, URL: asset.URL, SHA256: asset.SHA256}, nil
}

func secureArchiveTarget(destination, name string) (string, error) {
	name = filepath.Clean(strings.TrimPrefix(name, "./"))
	if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", errors.New("安装包包含越界路径")
	}
	target := filepath.Join(destination, name)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destination)+string(filepath.Separator)) {
		return "", errors.New("安装包包含越界路径")
	}
	return target, nil
}

func extractLinuxTar(reader io.Reader, destination string) error {
	tarReader := tar.NewReader(reader)
	var total int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := secureArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || total > maxNapcatExtractedSize-header.Size {
				return errors.New("QQ 运行时解压后超过 500 MB 限制")
			}
			total += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			handle, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode()|0o600)
			if err != nil {
				return err
			}
			count, copyErr := io.Copy(handle, io.LimitReader(tarReader, header.Size))
			closeErr := handle.Close()
			if copyErr != nil {
				return copyErr
			}
			if count != header.Size {
				return errors.New("QQ 运行时安装包内容不完整")
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("QQ 运行时安装包包含不受支持的链接或特殊文件")
		}
	}
}

func extractDebQQ(archivePath, destination string) error {
	handle, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer handle.Close()
	magic := make([]byte, 8)
	if _, err := io.ReadFull(handle, magic); err != nil || string(magic) != "!<arch>\n" {
		return errors.New("Linux QQ DEB 安装包格式无效")
	}
	for {
		header := make([]byte, 60)
		if _, err := io.ReadFull(handle, header); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		name := strings.TrimSpace(strings.TrimSuffix(string(header[:16]), "/"))
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil || size < 0 || size > maxNapcatArchiveSize {
			return errors.New("Linux QQ DEB 成员大小无效")
		}
		member := io.LimitReader(handle, size)
		if strings.HasPrefix(name, "data.tar.") {
			switch {
			case strings.HasSuffix(name, ".gz"):
				reader, err := gzip.NewReader(member)
				if err != nil {
					return err
				}
				err = extractLinuxTar(reader, destination)
				_ = reader.Close()
				return err
			case strings.HasSuffix(name, ".xz"):
				reader, err := xz.NewReader(member)
				if err != nil {
					return err
				}
				return extractLinuxTar(reader, destination)
			case strings.HasSuffix(name, ".zst"):
				reader, err := zstd.NewReader(member)
				if err != nil {
					return err
				}
				defer reader.Close()
				return extractLinuxTar(reader, destination)
			default:
				return fmt.Errorf("暂不支持 Linux QQ DEB 数据压缩格式 %s", name)
			}
		}
		if _, err := io.Copy(io.Discard, member); err != nil {
			return err
		}
		if size%2 != 0 {
			if _, err := handle.Seek(1, io.SeekCurrent); err != nil {
				return err
			}
		}
	}
	return errors.New("Linux QQ DEB 安装包缺少 data.tar 内容")
}

func parseCPIOHex(value []byte) (int64, error) {
	parsed, err := strconv.ParseInt(string(value), 16, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("RPM CPIO 头部无效")
	}
	return parsed, nil
}

func discardAligned(reader *bufio.Reader, count int64) error {
	padding := (4 - (count % 4)) % 4
	if padding == 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, reader, padding)
	return err
}

func extractNewcCPIO(reader io.Reader, destination string) error {
	buffered := bufio.NewReader(reader)
	var total int64
	for {
		header := make([]byte, 110)
		if _, err := io.ReadFull(buffered, header); err != nil {
			return err
		}
		if string(header[:6]) != "070701" && string(header[:6]) != "070702" {
			return errors.New("RPM CPIO 内容格式无效")
		}
		mode, err := parseCPIOHex(header[14:22])
		if err != nil {
			return err
		}
		fileSize, err := parseCPIOHex(header[54:62])
		if err != nil {
			return err
		}
		nameSize, err := parseCPIOHex(header[94:102])
		if err != nil || nameSize < 2 || nameSize > 4096 {
			return errors.New("RPM CPIO 文件名无效")
		}
		name := make([]byte, nameSize)
		if _, err := io.ReadFull(buffered, name); err != nil {
			return err
		}
		if err := discardAligned(buffered, nameSize); err != nil {
			return err
		}
		path := strings.TrimSuffix(string(name), "\x00")
		if path == "TRAILER!!!" {
			return nil
		}
		target, err := secureArchiveTarget(destination, path)
		if err != nil {
			return err
		}
		fileType := mode & 0o170000
		switch fileType {
		case 0o040000:
			err = os.MkdirAll(target, os.FileMode(mode&0o777)|0o700)
		case 0o100000:
			if total > maxNapcatExtractedSize-fileSize {
				return errors.New("QQ 运行时解压后超过 500 MB 限制")
			}
			total += fileSize
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				var handle *os.File
				handle, err = os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode&0o777)|0o600)
				if err == nil {
					count, copyErr := io.Copy(handle, io.LimitReader(buffered, fileSize))
					closeErr := handle.Close()
					if copyErr != nil {
						return copyErr
					}
					if count != fileSize {
						return errors.New("QQ RPM 内容不完整")
					}
					err = closeErr
				}
			}
		default:
			return errors.New("QQ RPM 包含不受支持的链接或特殊文件")
		}
		if err != nil {
			return err
		}
		if fileType != 0o100000 {
			if _, err := io.CopyN(io.Discard, buffered, fileSize); err != nil {
				return err
			}
		}
		if err := discardAligned(buffered, fileSize); err != nil {
			return err
		}
	}
}

const rpmPayloadCompressorTag = 1125

func readRPMHeader(reader io.Reader) (string, error) {
	header := make([]byte, 16)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", err
	}
	if string(header[:3]) != "\x8e\xad\xe8" || header[3] != 1 {
		return "", errors.New("RPM 头部格式无效")
	}
	entries := binary.BigEndian.Uint32(header[8:12])
	storeSize := binary.BigEndian.Uint32(header[12:16])
	if entries > 4096 || storeSize > uint32(maxNapcatArchiveSize) {
		return "", errors.New("RPM 头部大小无效")
	}
	index := make([]byte, int(entries)*16)
	if _, err := io.ReadFull(reader, index); err != nil {
		return "", err
	}
	store := make([]byte, int(storeSize))
	if _, err := io.ReadFull(reader, store); err != nil {
		return "", err
	}
	for offset := 0; offset < len(index); offset += 16 {
		if binary.BigEndian.Uint32(index[offset:offset+4]) != rpmPayloadCompressorTag {
			continue
		}
		valueOffset := binary.BigEndian.Uint32(index[offset+8 : offset+12])
		if valueOffset >= uint32(len(store)) {
			return "", errors.New("RPM 压缩格式字段无效")
		}
		value := store[valueOffset:]
		if end := strings.IndexByte(string(value), 0); end >= 0 {
			return strings.ToLower(string(value[:end])), nil
		}
		return "", errors.New("RPM 压缩格式字段无效")
	}
	return "", nil
}

func alignRPMHeader(file *os.File) error {
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	padding := (8 - (position % 8)) % 8
	if padding == 0 {
		return nil
	}
	_, err = file.Seek(padding, io.SeekCurrent)
	return err
}

func extractRPMQQ(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	lead := make([]byte, 96)
	if _, err := io.ReadFull(file, lead); err != nil {
		return err
	}
	if string(lead[:4]) != "\xed\xab\xee\xdb" {
		return errors.New("Linux QQ RPM 安装包格式无效")
	}
	if _, err := readRPMHeader(file); err != nil {
		return fmt.Errorf("读取 RPM 签名头失败：%w", err)
	}
	if err := alignRPMHeader(file); err != nil {
		return err
	}
	compressor, err := readRPMHeader(file)
	if err != nil {
		return fmt.Errorf("读取 RPM 元数据失败：%w", err)
	}
	var payload io.Reader = file
	switch compressor {
	case "", "none":
	case "gzip":
		reader, gzipErr := gzip.NewReader(file)
		if gzipErr != nil {
			return gzipErr
		}
		defer reader.Close()
		payload = reader
	case "xz":
		reader, xzErr := xz.NewReader(file)
		if xzErr != nil {
			return xzErr
		}
		payload = reader
	case "zstd", "zst":
		reader, zstdErr := zstd.NewReader(file)
		if zstdErr != nil {
			return zstdErr
		}
		defer reader.Close()
		payload = reader
	default:
		return fmt.Errorf("不支持 Linux QQ RPM 压缩格式 %s", compressor)
	}
	return extractNewcCPIO(payload, destination)
}

func patchLinuxQQEntrypoint(root, runtimeRoot string) error {
	packagePath := filepath.Join(root, "opt", "QQ", "resources", "app", "package.json")
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return fmt.Errorf("Linux QQ 运行时缺少 package.json：%w", err)
	}
	const original = `"main": "./application.asar/app_launcher/index.js"`
	if !strings.Contains(string(data), original) {
		return errors.New("Linux QQ package.json 的启动入口与受支持版本不匹配")
	}
	patched := strings.Replace(string(data), original, `"main": "./loadNapCat.js"`, 1)
	if err := os.WriteFile(packagePath+".alx-original", data, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(packagePath+".new", []byte(patched), 0o600); err != nil {
		return err
	}
	if err := os.Rename(packagePath+".new", packagePath); err != nil {
		return err
	}
	entrypoint := filepath.Join(root, "opt", "QQ", "resources", "app", "loadNapCat.js")
	content := "const path = require('path');\nconst home = process.env.NAPCAT_HOME || " + strconv.Quote(runtimeRoot) + ";\n(async () => { await import('file://' + path.join(home, 'napcat', 'napcat.mjs')); })();\n"
	return os.WriteFile(entrypoint, []byte(content), 0o600)
}
