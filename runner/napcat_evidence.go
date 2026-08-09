package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// napcatReleaseValidationEvidence is injected by release CI. An empty value
// deliberately keeps every automatic NapCat operation locked.
var napcatReleaseValidationEvidence = ""

type napcatEvidence struct {
	Platform           string `json:"platform"`
	Tag                string `json:"tag,omitempty"`
	Asset              string `json:"asset,omitempty"`
	ArchiveSHA256      string `json:"archiveSha256,omitempty"`
	InstallerCommit    string `json:"installerCommit,omitempty"`
	InstallerSHA256    string `json:"installerSha256,omitempty"`
	RuntimeFingerprint string `json:"runtimeFingerprint,omitempty"`
	ValidatedAt        string `json:"validatedAt"`
	ProcessModel       string `json:"processModel"`
	WebSocketRequired  bool   `json:"websocketRequired"`
}

func napcatEvidenceRecord() (napcatEvidence, error) {
	if strings.TrimSpace(napcatReleaseValidationEvidence) == "" {
		return napcatEvidence{}, errors.New("NapCat 当前平台尚无真实验证证据")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(napcatReleaseValidationEvidence))
	if err != nil {
		return napcatEvidence{}, errors.New("NapCat 验证证据格式无效")
	}
	var records []napcatEvidence
	if err := json.Unmarshal(decoded, &records); err != nil {
		var record napcatEvidence
		if json.Unmarshal(decoded, &record) != nil {
			return napcatEvidence{}, errors.New("NapCat 验证证据无法解析")
		}
		records = []napcatEvidence{record}
	}
	platform := napcatPlatform()
	if platform == nil {
		return napcatEvidence{}, errors.New("当前平台不支持 NapCat")
	}
	for _, record := range records {
		if record.Platform == platform.Key {
			if record.ValidatedAt == "" || record.ProcessModel != "foreground" {
				return napcatEvidence{}, errors.New("NapCat 验证证据不完整")
			}
			return record, nil
		}
	}
	return napcatEvidence{}, errors.New("当前平台尚无 NapCat 验证证据")
}

func napcatVerified() bool {
	platform := napcatPlatform()
	if platform == nil || !platform.AutoInstall {
		return false
	}
	_, err := napcatEvidenceRecord()
	return err == nil
}

func napcatStateVerified(state State) bool {
	evidence, err := napcatEvidenceRecord()
	platform := napcatPlatform()
	if err != nil || platform == nil || !state.Managed || state.Platform != platform.Key || state.ValidatedAt != evidence.ValidatedAt {
		return false
	}
	if state.Fingerprint == "" || state.Fingerprint != evidence.RuntimeFingerprint {
		return false
	}
	if platform.Key == "windows-amd64" {
		return state.ReleaseTag == evidence.Tag && state.Asset == evidence.Asset && strings.EqualFold(state.ArchiveSHA256, evidence.ArchiveSHA256)
	}
	return state.ReleaseTag == evidence.InstallerCommit && strings.EqualFold(state.ArchiveSHA256, evidence.InstallerSHA256)
}

func requireManagedNapcat(state State, action string) error {
	if !state.Managed || state.InstallMode != "managed" {
		return errors.New("当前 NapCat 是外部关联实例；工作台不能" + action + "。请使用其原始管理方式")
	}
	if !napcatStateVerified(state) {
		return errors.New("当前 NapCat 未与真实验证证据匹配；已拒绝" + action)
	}
	return nil
}

func requireNapcatConfirmation(confirmed bool, action string) error {
	if !confirmed {
		return errors.New("请确认后再" + action)
	}
	return nil
}

func reportNapcatProgress(stage string, percent int, message string) {
	payload, err := json.Marshal(map[string]any{"stage": stage, "percent": percent, "message": message})
	if err == nil {
		_, _ = fmt.Fprintf(os.Stderr, "@alx-progress %s\n", payload)
	}
}
