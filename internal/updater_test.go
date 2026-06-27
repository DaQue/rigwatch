package internal

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "patch upgrade available", current: "v1.2.3", latest: "v1.2.4", want: true},
		{name: "minor upgrade available with missing patch", current: "1.2", latest: "1.3.0", want: true},
		{name: "same version with prefix mismatch", current: "1.2.3", latest: "v1.2.3", want: false},
		{name: "older latest is not upgrade", current: "v1.2.3", latest: "v1.2.2", want: false},
		{name: "ignores build metadata", current: "v1.2.3+local", latest: "v1.2.4+release", want: true},
		{name: "ignores prerelease suffix for base comparison", current: "v1.2.3-dev", latest: "v1.2.3", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareVersions(tt.current, tt.latest); got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
