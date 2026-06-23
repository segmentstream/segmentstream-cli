package update

import "testing"

func TestChecksumForAsset(t *testing.T) {
	checksums := []byte("abc123  segmentstream_darwin_arm64.tar.gz\nother  segmentstream_linux_amd64.tar.gz\n")

	got, err := checksumForAsset(checksums, "segmentstream_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("checksumForAsset failed: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("checksum = %q, want abc123", got)
	}
}

func TestChecksumForAssetMissing(t *testing.T) {
	_, err := checksumForAsset([]byte("abc123  other.tar.gz\n"), "segmentstream_darwin_arm64.tar.gz")
	if err == nil {
		t.Fatal("expected missing checksum error")
	}
}
