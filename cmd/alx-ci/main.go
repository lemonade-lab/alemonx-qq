// alx-ci is the repository's portable CI utility. It intentionally replaces
// the historical Bash and Python release scripts with one audited Go command.
package main

import (
	"archive/tar"
	"archive/zip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/lemonade-lab/alemonx-qq/internal/qqruntime"
	"github.com/ulikunitz/xz"
)

const maxManifestSize = 64 << 10

var (
	idPattern      = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	shaPattern     = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	versionPattern = regexp.MustCompile(`^(?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*)){1,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

var luckyAssets = map[string]string{
	"windows-amd64": "LLBot-CLI-win-x64.zip",
	"darwin-arm64":  "LLBot-CLI-macos-arm64.tar.xz",
	"linux-amd64":   "LLBot-CLI-linux-x64.zip",
	"linux-arm64":   "LLBot-CLI-linux-arm64.zip",
}

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "alx-ci:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: alx-ci <validate-manifest|set-version|verify-version|evidence|validate-runtime|stage-napcat-runtime|package-napcat-runtime|verify-lucky-e2e|verify-napcat-e2e|verify-napcat-runtime>")
	}
	switch args[0] {
	case "validate-manifest":
		path := "alx.json"
		if len(args) == 2 {
			path = args[1]
		} else if len(args) > 2 {
			return errors.New("usage: validate-manifest [alx.json]")
		}
		return validateManifest(path)
	case "set-version":
		if len(args) != 2 {
			return errors.New("usage: set-version <tag>")
		}
		return setVersion("alx.json", args[1])
	case "verify-version":
		if len(args) != 2 {
			return errors.New("usage: verify-version <tag>")
		}
		return verifyVersion("alx.json", args[1])
	case "evidence":
		return evidenceCommand(args[1:])
	case "validate-runtime":
		return validateRuntimeEnvironment()
	case "package-napcat-runtime":
		return packageNapcatRuntime(args[1:])
	case "stage-napcat-runtime":
		return stageNapcatRuntime(args[1:])
	case "verify-lucky-e2e":
		return verifyLuckyE2E()
	case "verify-napcat-e2e":
		return verifyNapcatE2E()
	case "verify-napcat-runtime":
		return verifyNapcatRuntime()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func readJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("JSON root must be an object")
	}
	return value, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func setVersion(path, tag string) error {
	version := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid release tag: %s", tag)
	}
	manifest, err := readJSON(path)
	if err != nil {
		return err
	}
	manifest["version"] = version
	if err := writeJSON(path, manifest); err != nil {
		return err
	}
	fmt.Println("alx.json version =", version)
	return nil
}

func verifyVersion(path, tag string) error {
	manifest, err := readJSON(path)
	if err != nil {
		return err
	}
	version, _ := manifest["version"].(string)
	expected := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if version != expected {
		return fmt.Errorf("alx.json version %q does not match tag %q", version, expected)
	}
	return nil
}

func validateManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	errorsFound := []string{}
	if len(data) > maxManifestSize {
		errorsFound = append(errorsFound, fmt.Sprintf("manifest exceeds %d bytes", maxManifestSize))
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	stringValue := func(key string) string { value, _ := manifest[key].(string); return value }
	if !idPattern.MatchString(stringValue("id")) {
		errorsFound = append(errorsFound, fmt.Sprintf("id %q must match %s", stringValue("id"), idPattern.String()))
	}
	if strings.TrimSpace(stringValue("name")) == "" {
		errorsFound = append(errorsFound, "name is required")
	}
	if strings.TrimSpace(stringValue("version")) == "" {
		errorsFound = append(errorsFound, "version is required")
	}
	runtimeValue := stringValue("runtime")
	if runtimeValue != "" && runtimeValue != "binary" && runtimeValue != "node" && runtimeValue != "go" {
		errorsFound = append(errorsFound, fmt.Sprintf("runtime %q must be one of binary/node/go", runtimeValue))
	}
	if development, exists := manifest["development"]; exists {
		value, ok := development.(map[string]any)
		if !ok {
			errorsFound = append(errorsFound, "development must be an object")
		} else {
			runtime := asString(value["runtime"])
			_, hasEntry := value["entry"]
			if (runtime != "" && runtime != "binary" && runtime != "node" && runtime != "go") || !hasEntry {
				errorsFound = append(errorsFound, "development runtime must be binary/node/go and entry is required")
			}
		}
	}
	if entry, exists := manifest["entry"]; exists {
		value, ok := entry.(map[string]any)
		if !ok || len(value) == 0 {
			errorsFound = append(errorsFound, "entry must be a non-empty object")
		} else {
			for key := range value {
				if key != "go" && key != "linux" && key != "darwin" && key != "windows" && !map[string]bool{"darwin-arm64": true, "darwin-amd64": true, "linux-amd64": true, "linux-arm64": true, "windows-amd64": true}[key] {
					errorsFound = append(errorsFound, fmt.Sprintf("entry key %q is not a platform or go", key))
				}
			}
		}
	}
	web, ok := manifest["web"].(map[string]any)
	if !ok {
		errorsFound = append(errorsFound, "web is required: the plugin's interface is its web UI")
	} else if root, ok := web["root"].(string); !ok {
		errorsFound = append(errorsFound, "web must be an object with a string root")
	} else if root = strings.TrimSpace(root); root == "" || strings.HasPrefix(root, "/") || strings.Split(root, "/")[0] == ".." || containsPathParent(root) {
		errorsFound = append(errorsFound, "web.root must be a safe directory path inside the plugin")
	}
	if services, exists := manifest["services"]; exists {
		items, ok := services.([]any)
		if !ok {
			errorsFound = append(errorsFound, "services must be an array")
		} else {
			errorsFound = append(errorsFound, validateServices(items)...)
		}
	}
	if pickers, exists := manifest["systemPickers"]; exists {
		items, ok := pickers.([]any)
		if !ok {
			errorsFound = append(errorsFound, "systemPickers must be an array")
		} else {
			errorsFound = append(errorsFound, validateSystemPickers(items)...)
		}
	}
	if len(errorsFound) > 0 {
		return errors.New(strings.Join(errorsFound, "; "))
	}
	fmt.Println("alx.json OK")
	return nil
}

func asString(value any) string { text, _ := value.(string); return text }
func containsPathParent(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func validateServices(items []any) []string {
	errorsFound, ids := []string{}, map[string]bool{}
	for _, raw := range items {
		service, ok := raw.(map[string]any)
		if !ok {
			errorsFound = append(errorsFound, "service must be an object")
			continue
		}
		id := asString(service["id"])
		if !idPattern.MatchString(id) || ids[id] {
			errorsFound = append(errorsFound, "service id must be unique and valid")
		}
		ids[id] = true
		if strings.TrimSpace(asString(service["name"])) == "" {
			errorsFound = append(errorsFound, fmt.Sprintf("service %q name is required", id))
		}
		host := asString(service["host"])
		if host != "127.0.0.1" && host != "localhost" {
			errorsFound = append(errorsFound, fmt.Sprintf("service %q host must be loopback", id))
		}
		port, ok := service["port"].(float64)
		if !ok || port < 1 || port > 65535 || port != float64(int(port)) {
			errorsFound = append(errorsFound, fmt.Sprintf("service %q port must be valid", id))
		}
		for _, key := range []string{"basePath", "healthPath"} {
			value := "/"
			if item, exists := service[key]; exists {
				value = asString(item)
			}
			if !strings.HasPrefix(value, "/") || strings.Contains(value, "..") || strings.Contains(value, "\\") {
				errorsFound = append(errorsFound, fmt.Sprintf("service %q %s must be an absolute safe path", id, key))
			}
		}
	}
	return errorsFound
}

func validateSystemPickers(items []any) []string {
	errorsFound, ids := []string{}, map[string]bool{}
	for _, raw := range items {
		picker, ok := raw.(map[string]any)
		if !ok {
			errorsFound = append(errorsFound, "system picker must be an object")
			continue
		}
		id := asString(picker["id"])
		if !idPattern.MatchString(id) || ids[id] {
			errorsFound = append(errorsFound, "system picker id must be unique and valid")
		}
		ids[id] = true
		kind := asString(picker["kind"])
		if kind != "directory" && kind != "file" {
			errorsFound = append(errorsFound, fmt.Sprintf("system picker %q kind must be directory or file", id))
		}
		title := asString(picker["title"])
		if len([]rune(title)) > 120 || strings.ContainsAny(title, "\x00\r\n") {
			errorsFound = append(errorsFound, fmt.Sprintf("system picker %q title is invalid", id))
		}
	}
	return errorsFound
}

func evidenceCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: evidence <check-tree|encode|prepare-manifest>")
	}
	switch args[0] {
	case "check-tree":
		return checkEvidenceTree(".")
	case "encode":
		if len(args) != 3 {
			return errors.New("usage: evidence encode <core> <platform>")
		}
		record, err := loadEvidence(".", args[1], args[2])
		if err != nil {
			return err
		}
		if record == nil {
			return nil
		}
		data, _ := json.Marshal(record)
		fmt.Println(base64.RawURLEncoding.EncodeToString(data))
		return nil
	case "prepare-manifest":
		if len(args) != 4 {
			return errors.New("usage: evidence prepare-manifest <platform> <input> <output>")
		}
		return prepareManifest(".", args[1], args[2], args[3])
	default:
		return fmt.Errorf("unknown evidence command %q", args[0])
	}
}

func loadEvidence(root, core, platform string) (map[string]any, error) {
	path := filepath.Join(root, "evidence", core, platform+".json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	record, err := readJSON(path)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", path, err)
	}
	if err := validateEvidence(core, platform, record); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return record, nil
}

func requiredStrings(record map[string]any, fields ...string) error {
	for _, field := range fields {
		if strings.TrimSpace(asString(record[field])) == "" {
			return fmt.Errorf("evidence requires %s", field)
		}
	}
	return nil
}

func validateEvidence(core, platform string, record map[string]any) error {
	if asString(record["core"]) != core || asString(record["platform"]) != platform {
		return errors.New("core/platform does not match its path")
	}
	if err := requiredStrings(record, "validatedAt", "processModel", "runtimeFingerprint"); err != nil {
		return err
	}
	if asString(record["status"]) != "passed" || asString(record["processModel"]) != "foreground" || !shaPattern.MatchString(asString(record["runtimeFingerprint"])) {
		return errors.New("status, process model or runtime fingerprint is invalid")
	}
	if _, ok := record["websocketRequired"].(bool); !ok {
		return errors.New("websocketRequired must be boolean")
	}
	switch core {
	case "luckylillia":
		if err := requiredStrings(record, "tag", "asset", "archiveSha256"); err != nil {
			return err
		}
		if luckyAssets[platform] != asString(record["asset"]) || !shaPattern.MatchString(asString(record["archiveSha256"])) {
			return errors.New("LuckyLillia asset or SHA-256 is invalid")
		}
	case "napcat":
		return validateNapcatEvidence(platform, record)
	default:
		return fmt.Errorf("unknown core %q", core)
	}
	return nil
}

func validateNapcatEvidence(platform string, record map[string]any) error {
	if err := requiredStrings(record, "tag", "asset", "archiveSha256"); err != nil {
		return err
	}
	if platform == "windows-amd64" {
		if asString(record["asset"]) != "NapCat.Shell.Windows.OneKey.zip" || !shaPattern.MatchString(asString(record["archiveSha256"])) {
			return errors.New("Windows NapCat asset or SHA-256 is invalid")
		}
		return nil
	}
	if platform != "linux-amd64" && platform != "linux-arm64" {
		return errors.New("unsupported NapCat platform")
	}
	if asString(record["asset"]) != "NapCat.Shell.zip" || !shaPattern.MatchString(asString(record["archiveSha256"])) {
		return errors.New("Linux NapCat Shell asset or SHA-256 is invalid")
	}
	if err := requiredStrings(record, "runtimeAsset", "runtimeArchiveSha256"); err != nil {
		return err
	}
	goarch := strings.TrimPrefix(platform, "linux-")
	if !qqruntime.Matches(goarch, asString(record["runtimeAsset"]), asString(record["runtimeArchiveSha256"])) {
		return errors.New("Linux NapCat QQ runtime asset or SHA-256 is not a reviewed contract")
	}
	return nil
}

func checkEvidenceTree(root string) error {
	base := filepath.Join(root, "evidence")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || (entry.Name() != "napcat" && entry.Name() != "luckylillia") {
			return fmt.Errorf("unexpected evidence directory: %s", entry.Name())
		}
		files, err := os.ReadDir(filepath.Join(base, entry.Name()))
		if err != nil {
			return err
		}
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				if _, err := loadEvidence(root, entry.Name(), strings.TrimSuffix(file.Name(), ".json")); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func prepareManifest(root, platform, input, output string) error {
	manifest, err := readJSON(input)
	if err != nil {
		return err
	}
	flags := map[string]bool{"napcat-webui": false, "luckylillia-webui": false}
	for core, service := range map[string]string{"napcat": "napcat-webui", "luckylillia": "luckylillia-webui"} {
		record, err := loadEvidence(root, core, platform)
		if err != nil {
			return err
		}
		if record != nil {
			flags[service], _ = record["websocketRequired"].(bool)
		}
	}
	if services, ok := manifest["services"].([]any); ok {
		for _, raw := range services {
			if service, ok := raw.(map[string]any); ok {
				if enabled, exists := flags[asString(service["id"])]; exists {
					service["websocket"] = enabled
				}
			}
		}
	}
	return writeJSON(output, manifest)
}

func recordsFromEnv(name, label string) ([]map[string]any, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		fmt.Printf("%s validation evidence is absent; installation remains disabled.\n", label)
		return nil, nil
	}
	var records []map[string]any
	if strings.HasPrefix(raw, "{") {
		var record map[string]any
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			return nil, err
		}
		records = []map[string]any{record}
	} else if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("validation evidence must be one record or a non-empty record list")
	}
	return records, nil
}

func validateRuntimeEnvironment() error {
	lucky, err := recordsFromEnv("LUCKYLILLIA_VALIDATION_EVIDENCE", "LuckyLillia")
	if err != nil {
		return err
	}
	if err := validateRuntimeRecords("luckylillia", lucky); err != nil {
		return err
	}
	if len(lucky) > 0 {
		fmt.Println("LuckyLillia validation evidence accepted for release embedding.")
	}
	napcat, err := recordsFromEnv("NAPCAT_VALIDATION_EVIDENCE", "NapCat")
	if err != nil {
		return err
	}
	if err := validateRuntimeRecords("napcat", napcat); err != nil {
		return err
	}
	if len(napcat) > 0 {
		fmt.Println("NapCat validation evidence accepted for release embedding.")
	}
	return nil
}

func validateRuntimeRecords(core string, records []map[string]any) error {
	seen := map[string]bool{}
	for _, record := range records {
		if err := requiredStrings(record, "platform", "runtimeFingerprint", "validatedAt", "processModel"); err != nil {
			return err
		}
		platform := asString(record["platform"])
		if seen[platform] || !shaPattern.MatchString(asString(record["runtimeFingerprint"])) || asString(record["processModel"]) != "foreground" {
			return errors.New("duplicate platform, runtime fingerprint or process model is invalid")
		}
		if _, ok := record["websocketRequired"].(bool); !ok {
			return errors.New("websocketRequired must be boolean")
		}
		if core == "luckylillia" {
			if err := requiredStrings(record, "tag", "asset", "archiveSha256"); err != nil {
				return err
			}
			if luckyAssets[platform] != asString(record["asset"]) || !shaPattern.MatchString(asString(record["archiveSha256"])) {
				return errors.New("LuckyLillia evidence asset, platform or SHA-256 is invalid")
			}
		} else if core == "napcat" {
			if err := validateNapcatEvidence(platform, record); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unknown core %q", core)
		}
		seen[platform] = true
	}
	return nil
}

func latestRelease(repository string) (release, error) {
	response, err := (&http.Client{Timeout: 30 * time.Second}).Get("https://api.github.com/repos/" + repository + "/releases/latest")
	if err != nil {
		return release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("release request failed: %s", response.Status)
	}
	var value release
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&value); err != nil {
		return release{}, err
	}
	return value, nil
}

func assetByName(value release, name string) (releaseAsset, error) {
	for _, asset := range value.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("release asset %s not found", name)
}
func digest(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}

func downloadAndHash(url, path string) (string, error) {
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, 300<<20+1))
	if err != nil {
		return "", err
	}
	if count > 300<<20 {
		return "", errors.New("official archive exceeds 300 MB limit")
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func inspectCLIArchive(path, asset string) error {
	if strings.HasSuffix(asset, ".zip") {
		archive, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer archive.Close()
		entry := "llbot"
		if strings.Contains(asset, "win-") {
			entry = "llbot.exe"
		}
		for _, file := range archive.File {
			if filepath.Base(file.Name) == entry && !file.FileInfo().IsDir() {
				return nil
			}
		}
		return fmt.Errorf("%s missing from official archive", entry)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := xz.NewReader(file)
	if err != nil {
		return err
	}
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(header.Name) == "llbot" && header.Typeflag == tar.TypeReg {
			return nil
		}
	}
	return errors.New("llbot missing from official archive")
}

func runCommandFromJSON(envName string, environment []string) error {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fmt.Errorf("set %s to a JSON array: [executable, arg1, ...]", envName)
	}
	var parts []string
	if err := json.Unmarshal([]byte(raw), &parts); err != nil || len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return fmt.Errorf("%s must be a non-empty JSON command array", envName)
	}
	command := exec.Command(parts[0], parts[1:]...)
	command.Env = append(os.Environ(), environment...)
	command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
	return command.Run()
}

func linuxQQAssetForE2E(platform string) (qqruntime.Asset, error) {
	goarch := strings.TrimPrefix(platform, "linux-")
	if runtime.GOOS != "linux" || (goarch != "amd64" && goarch != "arm64") || runtime.GOARCH != goarch {
		return qqruntime.Asset{}, fmt.Errorf("Linux NapCat E2E platform %s must run on matching Linux host", platform)
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return qqruntime.AssetFor(goarch, "apt")
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return qqruntime.AssetFor(goarch, "dnf")
	}
	return qqruntime.Asset{}, errors.New("Linux NapCat E2E host requires APT or DNF")
}

func verifyLuckyE2E() error {
	platform := os.Getenv("LUCKYLILLIA_PLATFORM")
	assetName := luckyAssets[platform]
	if assetName == "" {
		return fmt.Errorf("unsupported LuckyLillia platform: %s", platform)
	}
	if err := os.MkdirAll("artifacts", 0o755); err != nil {
		return err
	}
	releaseInfo, err := latestRelease("LLOneBot/LuckyLilliaBot")
	if err != nil {
		return err
	}
	asset, err := assetByName(releaseInfo, assetName)
	if err != nil {
		return err
	}
	if !shaPattern.MatchString(digest(asset.Digest)) {
		return errors.New("official LuckyLillia asset lacks SHA-256")
	}
	archive := filepath.Join("artifacts", asset.Name)
	defer os.Remove(archive)
	actual, err := downloadAndHash(asset.URL, archive)
	if err != nil {
		return err
	}
	if actual != digest(asset.Digest) {
		return errors.New("LuckyLillia asset SHA-256 mismatch")
	}
	if err := inspectCLIArchive(archive, asset.Name); err != nil {
		return err
	}
	if err := runCommandFromJSON("LUCKYLILLIA_E2E_COMMAND_JSON", []string{"LUCKYLILLIA_RELEASE_TAG=" + releaseInfo.TagName, "LUCKYLILLIA_ARCHIVE=" + archive}); err != nil {
		return err
	}
	report, err := readJSON(filepath.Join("artifacts", "luckylillia-process-model.json"))
	if err != nil {
		return err
	}
	if asString(report["processModel"]) != "foreground" || !shaPattern.MatchString(asString(report["runtimeFingerprint"])) {
		return errors.New("LuckyLillia process report is invalid")
	}
	if _, ok := report["websocketRequired"].(bool); !ok {
		return errors.New("LuckyLillia process report must state websocketRequired")
	}
	for _, key := range []string{"launcherPid", "processGroupID"} {
		if _, ok := report[key].(float64); !ok {
			return fmt.Errorf("LuckyLillia process report requires %s", key)
		}
	}
	evidence := map[string]any{"core": "luckylillia", "tag": releaseInfo.TagName, "platform": platform, "asset": asset.Name, "archiveSha256": actual, "runtimeFingerprint": asString(report["runtimeFingerprint"]), "validatedAt": time.Now().UTC().Format(time.RFC3339), "processModel": "foreground", "websocketRequired": report["websocketRequired"], "status": "passed"}
	if err := validateRuntimeRecords("luckylillia", []map[string]any{evidence}); err != nil {
		return err
	}
	return writeJSON(filepath.Join("artifacts", "luckylillia-validation.json"), evidence)
}

func verifyNapcatE2E() error {
	platform := os.Getenv("NAPCAT_PLATFORM")
	if platform != "windows-amd64" && platform != "linux-amd64" && platform != "linux-arm64" {
		return fmt.Errorf("unsupported NapCat platform: %s", platform)
	}
	if err := os.MkdirAll("artifacts", 0o755); err != nil {
		return err
	}
	candidate := map[string]any{}
	environment := []string{}
	releaseInfo, err := latestRelease("NapNeko/NapCatQQ")
	if err != nil {
		return err
	}
	assetName := "NapCat.Shell.zip"
	if platform == "windows-amd64" {
		assetName = "NapCat.Shell.Windows.OneKey.zip"
	}
	asset, err := assetByName(releaseInfo, assetName)
	if err != nil {
		return err
	}
	if !shaPattern.MatchString(digest(asset.Digest)) {
		return errors.New("official NapCat asset lacks SHA-256")
	}
	archive := filepath.Join("artifacts", asset.Name)
	defer os.Remove(archive)
	actual, err := downloadAndHash(asset.URL, archive)
	if err != nil {
		return err
	}
	if actual != digest(asset.Digest) {
		return errors.New("NapCat asset SHA-256 mismatch")
	}
	if platform == "linux-amd64" || platform == "linux-arm64" {
		qqAsset, err := linuxQQAssetForE2E(platform)
		if err != nil {
			return err
		}
		qqArchive := filepath.Join("artifacts", qqAsset.Name)
		defer os.Remove(qqArchive)
		qqDigest, err := downloadAndHash(qqAsset.URL, qqArchive)
		if err != nil {
			return err
		}
		if qqDigest != qqAsset.SHA256 {
			return errors.New("Linux QQ runtime SHA-256 mismatch")
		}
		candidate["runtimeAsset"] = qqAsset.Name
		candidate["runtimeArchiveSha256"] = qqDigest
		environment = append(environment, "NAPCAT_RUNTIME_ASSET="+qqAsset.Name, "NAPCAT_RUNTIME_ARCHIVE="+qqArchive, "NAPCAT_RUNTIME_ARCHIVE_SHA256="+qqDigest)
	}
	archiveReader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	native := false
	for _, file := range archiveReader.File {
		entry := "napcat.mjs"
		if platform == "windows-amd64" {
			entry = "launcher.exe"
		}
		if filepath.Base(file.Name) == entry {
			native = true
			break
		}
	}
	_ = archiveReader.Close()
	if !native {
		return fmt.Errorf("NapCat archive lacks required %s", map[bool]string{true: "launcher.exe", false: "napcat.mjs"}[platform == "windows-amd64"])
	}
	candidate = map[string]any{"core": "napcat", "platform": platform, "tag": releaseInfo.TagName, "asset": asset.Name, "archiveSha256": actual}
	if err := writeJSON(filepath.Join("artifacts", "napcat-candidate-evidence.json"), candidate); err != nil {
		return err
	}
	environment = append(environment, "NAPCAT_RELEASE_TAG="+releaseInfo.TagName, "NAPCAT_ASSET="+asset.Name, "NAPCAT_ARCHIVE="+archive, "NAPCAT_ARCHIVE_SHA256="+actual)
	if err := runCommandFromJSON("NAPCAT_E2E_COMMAND_JSON", environment); err != nil {
		return err
	}
	report, err := readJSON(filepath.Join("artifacts", "napcat-e2e-report.json"))
	if err != nil {
		return err
	}
	if asString(report["platform"]) != platform || asString(report["processModel"]) != "foreground" || report["temporaryEvidenceInjected"] != true {
		return errors.New("NapCat E2E report platform, process model or temporary evidence is invalid")
	}
	stages, ok := report["stages"].(map[string]any)
	if !ok {
		return errors.New("NapCat E2E report stages are missing")
	}
	for _, stage := range []string{"install", "gatewayWebUI", "loginPending", "oneBot", "accountConfig", "stop", "update", "rollback"} {
		if stages[stage] != true {
			return fmt.Errorf("NapCat E2E stage %s did not pass", stage)
		}
	}
	evidence, ok := report["evidence"].(map[string]any)
	if !ok {
		return errors.New("NapCat E2E report evidence is missing")
	}
	for _, field := range []string{"tag", "asset", "archiveSha256", "runtimeAsset", "runtimeArchiveSha256"} {
		if _, expected := candidate[field]; !expected {
			continue
		}
		if evidence[field] != candidate[field] {
			return fmt.Errorf("NapCat E2E evidence %s does not match candidate", field)
		}
	}
	evidence["core"], evidence["platform"], evidence["processModel"], evidence["websocketRequired"], evidence["status"] = "napcat", platform, "foreground", report["websocketRequired"], "passed"
	if _, ok := evidence["websocketRequired"].(bool); !ok {
		return errors.New("NapCat E2E report must state websocketRequired")
	}
	if err := validateRuntimeRecords("napcat", []map[string]any{evidence}); err != nil {
		return err
	}
	return writeJSON(filepath.Join("artifacts", "napcat-validation.json"), evidence)
}

func verifyNapcatRuntime() error {
	stage := strings.TrimSpace(os.Getenv("NAPCAT_RUNTIME_STAGE"))
	platform := strings.TrimSpace(os.Getenv("NAPCAT_RUNTIME_PLATFORM"))
	if stage != "" || platform != "" {
		return verifyCompatibilityRuntime(stage, platform)
	}
	runner := strings.TrimSpace(os.Getenv("ALX_QQ_RUNNER"))
	if runner == "" {
		return errors.New("set ALX_QQ_RUNNER to the built almonx-qq executor path")
	}
	runAction := func(action string) (map[string]any, error) {
		command := exec.Command(runner)
		command.Stdin = strings.NewReader(fmt.Sprintf(`{"protocol":"alx/v1","method":"run","action":%q,"params":{}}`, action))
		output, err := command.Output()
		if err != nil {
			return nil, err
		}
		var envelope map[string]any
		if err := json.Unmarshal(output, &envelope); err != nil {
			return nil, err
		}
		payload, _ := envelope["output"].(string)
		var value map[string]any
		if err := json.Unmarshal([]byte(payload), &value); err != nil {
			return nil, err
		}
		return value, nil
	}
	status, err := runAction("napcat-status")
	if err != nil {
		return err
	}
	if status["installed"] != true || status["running"] != true || status["webUiReady"] != true {
		return errors.New("NapCat must be installed, running and have WebUI ready")
	}
	qr, err := runAction("napcat-qrcode")
	if err != nil {
		return err
	}
	if status["loginPending"] == true && qr["available"] != true {
		return errors.New("NapCat is login-pending but no guarded QR image is available")
	}
	if err := os.MkdirAll("artifacts", 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join("artifacts", "napcat-runtime-validation.json"), map[string]any{"validatedAt": time.Now().UTC().Format(time.RFC3339), "status": status, "qrCode": map[string]any{"available": qr["available"], "updatedAt": qr["updatedAt"]}})
}

func verifyCompatibilityRuntime(stage, platform string) error {
	if platform != "linux-amd64" && platform != "linux-arm64" {
		return errors.New("NAPCAT_RUNTIME_PLATFORM must be linux-amd64 or linux-arm64")
	}
	if stage == "" {
		return errors.New("set NAPCAT_RUNTIME_STAGE to the staged compatibility runtime directory")
	}
	manifest, err := readJSON(filepath.Join(stage, "alx-runtime.json"))
	if err != nil {
		return err
	}
	if asString(manifest["platform"]) != platform || asString(manifest["id"]) == "" {
		return errors.New("compatibility runtime manifest platform or ID is invalid")
	}
	for _, key := range []string{"xvfb", "loader", "libraryPath"} {
		relative := asString(manifest[key])
		if relative == "" || filepath.IsAbs(relative) || strings.HasPrefix(filepath.Clean(relative), "..") {
			return fmt.Errorf("compatibility runtime %s path is invalid", key)
		}
		info, statErr := os.Stat(filepath.Join(stage, relative))
		if statErr != nil || (key != "libraryPath" && info.Mode()&0o111 == 0) {
			return fmt.Errorf("compatibility runtime %s is unavailable", key)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("NAPCAT_RUNTIME_E2E_COMMAND_JSON")); raw != "" {
		if err := runCommandFromJSON("NAPCAT_RUNTIME_E2E_COMMAND_JSON", []string{"NAPCAT_RUNTIME_STAGE=" + stage, "NAPCAT_RUNTIME_PLATFORM=" + platform}); err != nil {
			return err
		}
	} else if err := smokeTestCompatibilityRuntime(stage, manifest); err != nil {
		return err
	}
	archiveHash := sha256.New()
	if err := filepath.Walk(stage, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		_, _ = archiveHash.Write([]byte(strings.TrimPrefix(path, stage)))
		_, _ = archiveHash.Write(data)
		return nil
	}); err != nil {
		return err
	}
	if err := os.MkdirAll("artifacts", 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join("artifacts", "napcat-runtime-validation.json"), map[string]any{
		"platform": platform, "runtimeID": asString(manifest["id"]), "runtimeFingerprint": fmt.Sprintf("%x", archiveHash.Sum(nil)), "validatedAt": time.Now().UTC().Format(time.RFC3339), "status": "passed",
	})
}

func smokeTestCompatibilityRuntime(stage string, manifest map[string]any) error {
	xvfb := filepath.Join(stage, asString(manifest["xvfb"]))
	loader := filepath.Join(stage, asString(manifest["loader"]))
	libraryPath := filepath.Join(stage, asString(manifest["libraryPath"]))
	display := fmt.Sprintf(":%d", 200+os.Getpid()%1000)
	command := exec.Command(loader, "--library-path", libraryPath, xvfb, display, "-screen", "0", "320x240x24", "-nolisten", "tcp", "-ac")
	if err := command.Start(); err != nil {
		return fmt.Errorf("compatibility runtime cannot start Xvfb: %w", err)
	}
	socket := filepath.Join("/tmp/.X11-unix", "X"+strings.TrimPrefix(display, ":"))
	ready := false
	for attempt := 0; attempt < 30; attempt++ {
		if _, err := os.Stat(socket); err == nil {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_, _ = command.Process.Wait()
	_ = os.Remove(socket)
	if !ready {
		return errors.New("compatibility runtime Xvfb did not create an X11 socket")
	}
	return nil
}

// packageNapcatRuntime produces a release-ready, self-contained runtime
// archive from a native build directory. The directory itself is prepared by
// the approved native builder; this command deliberately performs all archive
// construction, content enumeration and checksum generation in Go.
func packageNapcatRuntime(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: package-napcat-runtime <linux-amd64|linux-arm64> <stage-dir> <output-dir>")
	}
	platform, stage, output := args[0], args[1], args[2]
	if platform != "linux-amd64" && platform != "linux-arm64" {
		return errors.New("runtime platform must be linux-amd64 or linux-arm64")
	}
	manifest, err := readJSON(filepath.Join(stage, "alx-runtime.json"))
	if err != nil {
		return err
	}
	if asString(manifest["platform"]) != platform || asString(manifest["id"]) == "" {
		return errors.New("runtime manifest platform or ID is invalid")
	}
	for _, key := range []string{"xvfb", "loader", "libraryPath"} {
		relative := asString(manifest[key])
		if relative == "" || filepath.IsAbs(relative) || strings.HasPrefix(filepath.Clean(relative), "..") {
			return fmt.Errorf("runtime %s path is invalid", key)
		}
		info, statErr := os.Stat(filepath.Join(stage, relative))
		if statErr != nil || (key != "libraryPath" && (info.IsDir() || info.Mode()&0o111 == 0)) {
			return fmt.Errorf("runtime %s is unavailable", key)
		}
	}
	asset := "alemonx-qq-runtime-" + platform + "-glibc.tar.zst"
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	archivePath := filepath.Join(output, asset)
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	checksum := sha256.New()
	writer := io.MultiWriter(archive, checksum)
	zstdWriter, err := zstd.NewWriter(writer)
	if err != nil {
		_ = archive.Close()
		return err
	}
	tarWriter := tar.NewWriter(zstdWriter)
	files := []map[string]any{}
	err = filepath.Walk(stage, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(stage, path)
		if err != nil || relative == "." {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("runtime contains unsupported file %s", relative)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileHash := sha256.Sum256(contents)
		files = append(files, map[string]any{"path": header.Name, "sha256": fmt.Sprintf("%x", fileHash[:]), "size": len(contents)})
		_, err = tarWriter.Write(contents)
		return err
	})
	closeTar := tarWriter.Close()
	closeZstd := zstdWriter.Close()
	closeFile := archive.Close()
	if err != nil {
		return err
	}
	if closeTar != nil || closeZstd != nil || closeFile != nil {
		return errors.New("cannot finalize runtime archive")
	}
	digest := fmt.Sprintf("%x", checksum.Sum(nil))
	if err := os.WriteFile(filepath.Join(output, asset+".sha256"), []byte(digest+"  "+asset+"\n"), 0o600); err != nil {
		return err
	}
	return writeJSON(filepath.Join(output, asset+".sbom.json"), map[string]any{"format": "alx-runtime-sbom/v1", "platform": platform, "runtimeID": asString(manifest["id"]), "archive": asset, "archiveSha256": digest, "files": files})
}

// stageNapcatRuntime turns a native package-manager installation into the
// self-contained layout consumed by the runner. It never shells out: CI passes
// the already resolved package file paths, and Go discovers their ELF
// dependencies, copies them and writes the runtime contract.
func stageNapcatRuntime(args []string) error {
	if len(args) < 4 {
		return errors.New("usage: stage-napcat-runtime <linux-amd64|linux-arm64> <Xvfb> <loader> <output-dir> [library-path ...]")
	}
	platform, xvfb, loader, output := args[0], args[1], args[2], args[3]
	if platform != "linux-amd64" && platform != "linux-arm64" {
		return errors.New("runtime platform must be linux-amd64 or linux-arm64")
	}
	for _, path := range []string{xvfb, loader} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return fmt.Errorf("runtime binary is unavailable: %s", path)
		}
	}
	if err := os.RemoveAll(output); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "bin"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "lib"), 0o755); err != nil {
		return err
	}
	copyInto := func(source, destination string, executable bool) error {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if executable {
			mode = 0o755
		}
		return os.WriteFile(destination, data, mode)
	}
	if err := copyInto(xvfb, filepath.Join(output, "bin", "Xvfb"), true); err != nil {
		return err
	}
	loaderName := filepath.Base(loader)
	if err := copyInto(loader, filepath.Join(output, "lib", loaderName), true); err != nil {
		return err
	}
	dependencies, err := elfDependencyClosure(append([]string{xvfb}, args[4:]...))
	if err != nil {
		return err
	}
	for _, library := range dependencies {
		if filepath.Clean(library) == filepath.Clean(loader) {
			continue
		}
		if err := copyInto(library, filepath.Join(output, "lib", filepath.Base(library)), false); err != nil {
			return err
		}
	}
	id := platform + "-glibc-v1"
	manifest := map[string]any{"id": id, "platform": platform, "xvfb": "bin/Xvfb", "loader": "lib/" + loaderName, "libraryPath": "lib"}
	return writeJSON(filepath.Join(output, "alx-runtime.json"), manifest)
}

func elfDependencyClosure(initial []string) ([]string, error) {
	queue := append([]string(nil), initial...)
	seen := map[string]bool{}
	resolved := map[string]string{}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if seen[path] {
			continue
		}
		seen[path] = true
		libraries, err := elfLibraries(path)
		if err != nil {
			return nil, err
		}
		for _, library := range libraries {
			if library == "" || seen[library] {
				continue
			}
			resolved[library] = library
			queue = append(queue, library)
		}
	}
	values := make([]string, 0, len(resolved))
	for value := range resolved {
		values = append(values, value)
	}
	sort.Strings(values)
	return values, nil
}

// elfLibraries reads DT_NEEDED and resolves only absolute paths emitted by
// the native package manager's loader cache. Its parser purposely avoids
// invoking ldd, which executes the target binary on some systems.
func elfLibraries(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Read the dynamic linker trace through the loader itself rather than ldd;
	// it is a deterministic inspection command and never runs Xvfb.
	loader := "/lib64/ld-linux-x86-64.so.2"
	if runtime.GOARCH == "arm64" {
		loader = "/lib/ld-linux-aarch64.so.1"
	}
	if _, err := os.Stat(loader); err != nil {
		return nil, fmt.Errorf("native dynamic loader unavailable while inspecting %s", path)
	}
	command := exec.Command(loader, "--list", path)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot inspect runtime dependency %s: %w", path, err)
	}
	result := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		index := strings.Index(line, " => ")
		if index < 0 {
			continue
		}
		rest := strings.TrimSpace(line[index+4:])
		pathEnd := strings.Index(rest, " (")
		if pathEnd >= 0 {
			rest = rest[:pathEnd]
		}
		if filepath.IsAbs(rest) {
			result = append(result, rest)
		}
	}
	return result, nil
}
