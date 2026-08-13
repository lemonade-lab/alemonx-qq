package qqruntime

import "testing"

func TestAssetsHaveFixedPlatformContracts(t *testing.T) {
	for _, test := range []struct{ manager, goarch string }{
		{"apt", "amd64"}, {"apt", "arm64"}, {"dnf", "amd64"}, {"dnf", "arm64"},
	} {
		asset, err := AssetFor(test.goarch, test.manager)
		if err != nil || asset.Name == "" || asset.URL == "" || asset.Kind == "" {
			t.Fatalf("%s/%s = %#v, err=%v", test.manager, test.goarch, asset, err)
		}
	}
	if _, err := AssetFor("amd64", "pacman"); err == nil {
		t.Fatal("unsupported package manager must be rejected")
	}
}
