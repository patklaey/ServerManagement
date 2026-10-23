package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  version
	}{
		{"1.2.3", version{1, 2, 3}},
		{"v1.2.3", version{1, 2, 3}},
		{"10.20.30", version{10, 20, 30}},
		{"invalid", version{}},
	}

	for _, tt := range tests {
		got := parseVersion(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestIsPatchDiffOnly(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"1.2.3", "1.2.4", true},
		{"v1.2.3", "1.2.9", true},
		{"1.2.3", "1.3.0", false},
		{"1.2.3", "2.0.0", false},
	}

	for _, tt := range tests {
		got := isPatchDiffOnly(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("isPatchDiffOnly(%q, %q) = %v, want %v",
				tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestVersionSortDescending(t *testing.T) {
	input := []string{
		"1.2.3",
		"1.10.1",
		"v2.0.0",
		"1.9.9",
		"v1.10.2",
	}

	expected := []string{
		"v2.0.0",
		"v1.10.2",
		"1.10.1",
		"1.9.9",
		"1.2.3",
	}

	input = sortSematic(input)

	if !reflect.DeepEqual(input, expected) {
		t.Errorf("version sort result = %v, want %v", input, expected)
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1.2.3", true},
		{"v1.2.3", true},
		{"1.2", false},
		{"1.2.3.4", false},
		{"latest", false},
		{"1.2.x", false},
	}

	for _, tt := range tests {
		got := isValidVersion(tt.input)
		if got != tt.want {
			t.Errorf("isValidVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCanonicalLocalRepo(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"wordpress", "patklaey/wordpress"},
		{"library/wordpress", "patklaey/wordpress"},
		{"nginx", "patklaey/nginx"},
		{"library/nginx", "patklaey/nginx"},
		{"owncloud/server", "owncloud/server"},
		{"library/busybox", "busybox"},
	}

	for _, tt := range tests {
		got := canonicalLocalRepo(tt.input)
		if got != tt.want {
			t.Errorf("canonicalLocalRepo(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHandleSameTagSameDigestDoesNotAlert(t *testing.T) {
	mailMessage.Reset()
	defer mailMessage.Reset()

	repo := &ImageConfig{Name: "library/nginx", AlertOnDigestDiff: true, UpgradeOnDigestDiff: true}
	handleSameTag(repo, "1.2.3", "sha256:same", "sha256:same")

	if mailMessage.Len() != 0 {
		t.Errorf("expected no mail message for matching digests, got %q", mailMessage.String())
	}
}

func TestHandleSameTagDigestDiffAlertsWhenConfigured(t *testing.T) {
	mailMessage.Reset()
	defer mailMessage.Reset()

	repo := &ImageConfig{Name: "library/nginx", AlertOnDigestDiff: true}
	handleSameTag(repo, "1.2.3", "sha256:old", "sha256:new")

	msg := mailMessage.String()
	if !strings.Contains(msg, "library/nginx") || !strings.Contains(msg, "sha256:old") || !strings.Contains(msg, "sha256:new") {
		t.Errorf("expected mail message to mention repo and both digests, got %q", msg)
	}
}

func TestHandleSameTagDigestDiffWithoutAlertOrUpgradeIsSilent(t *testing.T) {
	mailMessage.Reset()
	defer mailMessage.Reset()

	repo := &ImageConfig{Name: "library/nginx"}
	handleSameTag(repo, "1.2.3", "sha256:old", "sha256:new")

	if mailMessage.Len() != 0 {
		t.Errorf("expected no mail message when digest checking is not configured, got %q", mailMessage.String())
	}
}

func TestHandleSameTagDigestDiffRunsUpgradeScript(t *testing.T) {
	mailMessage.Reset()
	defer mailMessage.Reset()

	repo := &ImageConfig{
		Name:                "library/nginx",
		UpgradeOnDigestDiff: true,
		UpgradeScript:       "echo rebuilding fromVersionNumber toVersionNumber",
	}
	handleSameTag(repo, "1.2.3", "sha256:old", "sha256:new")

	msg := mailMessage.String()
	if !strings.Contains(msg, "Successfully upgraded library/nginx from 1.2.3 to 1.2.3") {
		t.Errorf("expected mail message to record the rebuild as an upgrade, got %q", msg)
	}
}

func TestRunUpgradeScriptRecordsFailure(t *testing.T) {
	mailMessage.Reset()
	defer mailMessage.Reset()

	repo := &ImageConfig{Name: "library/nginx", UpgradeScript: "exit 1"}
	runUpgradeScript(repo, "1.2.3", "1.2.4")

	msg := mailMessage.String()
	if !strings.Contains(msg, "Error occurred while upgrading library/nginx") {
		t.Errorf("expected mail message to record the failure, got %q", msg)
	}
}
