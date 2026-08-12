// Package qqruntime owns the reviewed Tencent QQ runtime download contract
// shared by the installer and the E2E evidence tool.
package qqruntime

import "fmt"

const baseURL = "https://qqdl.gtimg.cn/qqfile/QQNT/9.9.32/release/c390e792/"

// Asset is one official Linux QQ runtime archive. SHA256 is pinned because
// Tencent's direct download endpoint does not provide a stable checksum file.
type Asset struct {
	Name   string
	URL    string
	Kind   string
	SHA256 string
}

var assets = map[string]Asset{
	"apt/amd64": {
		Name: "QQ_3.2.31_260710_amd64_01.deb", Kind: "deb",
		SHA256: "02f677feb1ce01ed293a3c7761e5dd85bd79936f57dcaa4cdb53178ae30e3d6d",
	},
	"apt/arm64": {
		Name: "QQ_3.2.31_260710_arm64_01.deb", Kind: "deb",
		SHA256: "ac604371f5c486acf6cbf83dd667e622ee1f487d0c8bd425627de6d68fe34974",
	},
	"dnf/amd64": {
		Name: "QQ_3.2.31_260710_x86_64_01.rpm", Kind: "rpm",
		SHA256: "be897976f9481be2d224dc4e11592126a3adf71b2c395e8273cf14ea99b5519d",
	},
	"dnf/arm64": {
		Name: "QQ_3.2.31_260710_aarch64_01.rpm", Kind: "rpm",
		SHA256: "0a48d0a82881ab6a6716b7f90250ecaab1305727e7b5bf2d16c9205cb0c28995",
	},
}

// AssetFor returns the exact reviewed runtime archive for a package manager
// and Go architecture. No caller may supply a URL or a checksum.
func AssetFor(goarch, packageManager string) (Asset, error) {
	asset, ok := assets[packageManager+"/"+goarch]
	if !ok {
		return Asset{}, fmt.Errorf("unsupported Linux QQ runtime: %s/%s", packageManager, goarch)
	}
	asset.URL = baseURL + asset.Name
	return asset, nil
}

// Matches reports whether a Linux evidence record describes one reviewed
// runtime archive for the supplied architecture, regardless of APT or DNF.
func Matches(goarch, name, sha256 string) bool {
	for _, manager := range []string{"apt", "dnf"} {
		asset, err := AssetFor(goarch, manager)
		if err == nil && asset.Name == name && asset.SHA256 == sha256 {
			return true
		}
	}
	return false
}
