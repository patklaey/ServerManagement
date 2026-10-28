package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// With no path configured, tracking is entirely disabled: set is a no-op
// rather than an in-memory-only cache. main relies on this by checking
// digests.path == "" up front and never calling get/set at all in that
// case, but set must stay harmless if it is ever called anyway.
func TestDigestStateWithoutPathIsANoOp(t *testing.T) {
	s := loadDigestState("")

	if got := s.get("library/nginx", "1.2.3"); got != "" {
		t.Errorf("get() on an unconfigured state = %q, want empty string", got)
	}

	s.set("library/nginx", "1.2.3", "sha256:abc")
	if got := s.get("library/nginx", "1.2.3"); got != "" {
		t.Errorf("get() after set() with no path configured = %q, want empty string (set should be a no-op)", got)
	}
}

func TestDigestStateMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	s := loadDigestState(path)
	if got := s.get("library/nginx", "1.2.3"); got != "" {
		t.Errorf("get() on a missing state file = %q, want empty string", got)
	}
}

func TestDigestStateSetPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest-state.json")

	s := loadDigestState(path)
	s.set("library/nginx", "1.2.3", "sha256:abc")
	s.set("library/wordpress", "6.4.2", "sha256:def")

	reloaded := loadDigestState(path)
	if got := reloaded.get("library/nginx", "1.2.3"); got != "sha256:abc" {
		t.Errorf("get(nginx) after reload = %q, want sha256:abc", got)
	}
	if got := reloaded.get("library/wordpress", "6.4.2"); got != "sha256:def" {
		t.Errorf("get(wordpress) after reload = %q, want sha256:def", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}
	var onDisk map[string]digestEntry
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}
	if onDisk["library/nginx"] != (digestEntry{Tag: "1.2.3", Digest: "sha256:abc"}) {
		t.Errorf("on-disk entry for library/nginx = %+v, want {1.2.3 sha256:abc}", onDisk["library/nginx"])
	}
}

func TestDigestStateSetOverwritesPreviousValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest-state.json")

	s := loadDigestState(path)
	s.set("library/nginx", "1.2.3", "sha256:old")
	s.set("library/nginx", "1.2.3", "sha256:new")

	if got := s.get("library/nginx", "1.2.3"); got != "sha256:new" {
		t.Errorf("get() after overwrite = %q, want sha256:new", got)
	}
}

// A version bump (e.g. 1.2.3 -> 1.3.0) must not grow the file: the new tag
// replaces the old tag's entry for that repo instead of adding a second
// one, and asking for the old tag's digest now reports it as unknown
// rather than returning what is actually the new tag's digest.
func TestDigestStateVersionBumpReplacesEntryInsteadOfGrowing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest-state.json")

	s := loadDigestState(path)
	s.set("library/nginx", "1.2.3", "sha256:old")
	s.set("library/nginx", "1.3.0", "sha256:new")

	if len(s.entries) != 1 {
		t.Fatalf("entries = %+v, want exactly one entry for library/nginx", s.entries)
	}
	if got := s.get("library/nginx", "1.2.3"); got != "" {
		t.Errorf("get() for the superseded tag = %q, want empty string (unknown)", got)
	}
	if got := s.get("library/nginx", "1.3.0"); got != "sha256:new" {
		t.Errorf("get() for the current tag = %q, want sha256:new", got)
	}
}

func TestDigestStatePruneRemovesUntrackedRepos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest-state.json")

	s := loadDigestState(path)
	s.set("library/nginx", "1.2.3", "sha256:abc")
	s.set("library/wordpress", "6.4.2", "sha256:def")

	s.prune(map[string]bool{"library/nginx": true})

	if got := s.get("library/nginx", "1.2.3"); got != "sha256:abc" {
		t.Errorf("get(nginx) after pruning = %q, want sha256:abc (should be kept)", got)
	}
	if got := s.get("library/wordpress", "6.4.2"); got != "" {
		t.Errorf("get(wordpress) after pruning = %q, want empty string (should have been removed)", got)
	}

	reloaded := loadDigestState(path)
	if got := reloaded.get("library/wordpress", "6.4.2"); got != "" {
		t.Errorf("get(wordpress) after reload = %q, want empty string (pruning should have persisted)", got)
	}
}

func TestDigestTrackedRepos(t *testing.T) {
	repos := []*ImageConfig{
		{Name: "library/nginx", AlertOnDigestDiff: true},
		{Name: "library/wordpress", UpgradeOnDigestDiff: true},
		{Name: "owncloud/server"},
	}

	got := digestTrackedRepos(repos)

	want := map[string]bool{"library/nginx": true, "library/wordpress": true}
	if len(got) != len(want) {
		t.Fatalf("digestTrackedRepos() = %v, want %v", got, want)
	}
	for repo := range want {
		if !got[repo] {
			t.Errorf("digestTrackedRepos() missing %q", repo)
		}
	}
}

func TestDigestStateIgnoresCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest-state.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("writing corrupt state file: %v", err)
	}

	s := loadDigestState(path)
	if got := s.get("library/nginx", "1.2.3"); got != "" {
		t.Errorf("get() on a corrupt state file = %q, want empty string", got)
	}
}
