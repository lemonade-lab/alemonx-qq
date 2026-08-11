package qqruntime

import "testing"

func TestReviewedAssetsHavePinnedHashes(t *testing.T) {
	for _, test := range []struct{ manager, goarch string }{
		{"apt", "amd64"}, {"apt", "arm64"}, {"dnf", "amd64"}, {"dnf", "arm64"},
	} {
		asset, err := AssetFor(test.goarch, test.manager)
		if err != nil || len(asset.SHA256) != 64 || asset.URL == "" || !Matches(test.goarch, asset.Name, asset.SHA256) {
			t.Fatalf("%s/%s = %#v, err=%v", test.manager, test.goarch, asset, err)
		}
	}
	if _, err := AssetFor("amd64", "pacman"); err == nil {
		t.Fatal("unsupported package manager must be rejected")
	}
}
