package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "equal", left: "0.1.0", right: "v0.1.0", want: 0},
		{name: "older", left: "0.1.0", right: "0.1.1", want: -1},
		{name: "newer", left: "1.0.0", right: "0.9.0", want: 1},
		{name: "prerelease older than stable", left: "1.0.0-rc1", right: "1.0.0", want: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := compareVersions(test.left, test.right)
			if err != nil {
				t.Fatalf("compareVersions returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestCompareVersionsRejectsInvalid(t *testing.T) {
	if _, err := compareVersions("dev", "0.1.0"); err == nil {
		t.Fatal("expected invalid version error")
	}
}
