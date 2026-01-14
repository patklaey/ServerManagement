package main

import (
	"reflect"
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
