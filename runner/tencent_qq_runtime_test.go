package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchCurrentLinuxQQAssetUsesMatchingArchitectureAndFallbackFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Linux":{"x64DownloadUrl":{"deb":"https://qqdl.gtimg.cn/qq/QQ_arm64.deb","rpm":"https://qqdl.gtimg.cn/qq/QQ_x86_64.rpm"}}}`))
	}))
	defer server.Close()
	asset, err := fetchLinuxQQAssetFromConfigURL(server.Client(), server.URL, "amd64", "deb")
	if err != nil || asset.Name != "QQ_x86_64.rpm" || asset.Kind != "rpm" {
		t.Fatalf("asset=%#v err=%v", asset, err)
	}
	if _, err := validateTencentQQAsset("https://qqdl.gtimg.cn/qq/QQ_arm64.deb", "amd64", "deb"); err == nil {
		t.Fatal("arm64 package must not be accepted for amd64")
	}
}
