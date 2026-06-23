package update

import "testing"

func TestAssetName(t *testing.T) {
	got := assetName("darwin", "arm64")
	if got != "segmentstream_darwin_arm64.tar.gz" {
		t.Fatalf("assetName = %q", got)
	}
}

func TestFindAsset(t *testing.T) {
	release := GitHubRelease{
		TagName: "v0.1.0",
		Assets:  []GitHubAsset{{Name: "checksums.txt"}, {Name: "segmentstream_linux_amd64.tar.gz"}},
	}

	asset, err := findAsset(release, "segmentstream_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("findAsset failed: %v", err)
	}
	if asset.Name != "segmentstream_linux_amd64.tar.gz" {
		t.Fatalf("asset name = %q", asset.Name)
	}
}
