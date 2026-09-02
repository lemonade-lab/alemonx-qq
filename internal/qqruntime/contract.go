// Package qqruntime owns the fixed Tencent QQ runtime download contract.
package qqruntime

import "fmt"

// Tencent's newer QQNTV2 links currently reject container downloads with 403.
// Keep a tested official QQNT release as the recovery contract; the installer
// tries Tencent's current metadata first and reaches this contract only if
// that download itself fails.
const baseURL = "https://qqdl.gtimg.cn/qqfile/QQNT/9.9.33/release/c97651b2/"

// Asset is one official Linux QQ runtime archive.
type Asset struct {
	Name string
	URL  string
	Kind string
}

var assets = map[string]Asset{
	"apt/amd64": {
		Name: "QQ_3.2.32_260730_amd64_01.deb", Kind: "deb",
	},
	"apt/arm64": {
		Name: "QQ_3.2.32_260730_arm64_01.deb", Kind: "deb",
	},
	"dnf/amd64": {
		Name: "QQ_3.2.32_260730_x86_64_01.rpm", Kind: "rpm",
	},
	"dnf/arm64": {
		Name: "QQ_3.2.32_260730_aarch64_01.rpm", Kind: "rpm",
	},
}

// AssetFor returns the fixed runtime archive for a package manager and Go architecture.
func AssetFor(goarch, packageManager string) (Asset, error) {
	asset, ok := assets[packageManager+"/"+goarch]
	if !ok {
		return Asset{}, fmt.Errorf("unsupported Linux QQ runtime: %s/%s", packageManager, goarch)
	}
	asset.URL = baseURL + asset.Name
	return asset, nil
}
