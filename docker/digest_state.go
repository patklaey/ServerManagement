package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// digestState persists the most recently observed registry digest for
// repos whose locally built image has no RepoDigest of its own (because it
// was `docker build`'t FROM a base image rather than pulled directly, e.g.
// a custom nginx image built FROM library/nginx). This lets handleSameTag
// still detect a same-tag rebuild of the base image across runs, by
// comparing the latest digest to the one we saw last time instead of to a
// local digest that doesn't exist.
//
// Each repo keeps exactly one entry, for whichever tag was current the
// last time it was recorded: a version bump overwrites it rather than
// adding a new one, so the file doesn't grow with every release ever seen.
// get returns "" (unknown) if the stored entry is for a different tag than
// asked about, e.g. right after such a version bump.
//
// On disk it is a JSON object keyed by repo: {"library/nginx": {"tag":
// "1.27.3", "digest": "sha256:..."}, ...}. If path is empty, state is kept
// in memory only and never persisted.
type digestState struct {
	path    string
	entries map[string]digestEntry
}

type digestEntry struct {
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

func loadDigestState(path string) *digestState {
	s := &digestState{path: path, entries: map[string]digestEntry{}}
	if path == "" {
		return s
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("%s: could not read digest state file %s: %v\n", date, path, err)
		}
		return s
	}

	if err := json.Unmarshal(data, &s.entries); err != nil {
		fmt.Printf("%s: could not parse digest state file %s: %v\n", date, path, err)
	}

	return s
}

// get returns the digest recorded for repo, or "" if none is known yet or
// the recorded entry is for a different tag (e.g. just after a version
// bump, when there is nothing yet to compare the new tag's digest to).
func (s *digestState) get(repo, tag string) string {
	e, ok := s.entries[repo]
	if !ok || e.Tag != tag {
		return ""
	}
	return e.Digest
}

// set records digest as the latest known value for repo, tag, replacing
// any entry for a previous tag, and persists the whole state file
// immediately so a crash partway through a run doesn't lose tracking for
// repos already processed. It is a no-op if no path was configured.
func (s *digestState) set(repo, tag, digest string) {
	if s.path == "" {
		return
	}
	s.entries[repo] = digestEntry{Tag: tag, Digest: digest}
	s.persist()
}

// prune drops entries for repos not in keep, e.g. ones removed or renamed
// in the config since the state file was last written, and persists the
// change if anything was removed. It is a no-op if no path was configured.
func (s *digestState) prune(keep map[string]bool) {
	if s.path == "" {
		return
	}

	changed := false
	for repo := range s.entries {
		if !keep[repo] {
			delete(s.entries, repo)
			changed = true
		}
	}

	if changed {
		s.persist()
	}
}

// digestTrackedRepos returns the set of repo names that rely on digestState
// (i.e. have digest checking enabled at all), for pruning entries for
// repos that no longer do.
func digestTrackedRepos(repos []*ImageConfig) map[string]bool {
	keep := make(map[string]bool, len(repos))
	for _, repo := range repos {
		if repo.AlertOnDigestDiff || repo.UpgradeOnDigestDiff {
			keep[repo.Name] = true
		}
	}
	return keep
}

func (s *digestState) persist() {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		fmt.Printf("%s: could not encode digest state: %v\n", date, err)
		return
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		fmt.Printf("%s: could not write digest state file %s: %v\n", date, s.path, err)
	}
}
