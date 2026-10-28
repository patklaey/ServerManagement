package main

import (
	"ServerManagement/utils"
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/moby/moby/client"
	"github.com/samber/lo"
)

type Config struct {
	Images         []*ImageConfig `yaml:"images"`
	MailConfigPath string         `yaml:"mailConfigPath"`
	// DigestStateFilePath is where the last-seen registry digest of each
	// repo is recorded, for images with no local digest of their own (e.g.
	// a custom image built FROM a base image rather than pulled). If
	// empty, such images cannot be digest-checked across runs. See
	// digestState.
	DigestStateFilePath string `yaml:"digestStateFilePath,omitempty"`
}
type ImageConfig struct {
	Name string `yaml:"name"`
	// Registry selects which registry Name is looked up on: "dockerhub"
	// (the default, if left empty) or "gcr". See registryClientFor.
	Registry            string `yaml:"registry,omitempty"`
	AlertOnPatchDiff    bool   `yaml:"alertOnPatchDiff"`
	UpgradeOnPatchDiff  bool   `yaml:"upgradeOnPatchDiff"`
	AlertOnDigestDiff   bool   `yaml:"alertOnDigestDiff"`
	UpgradeOnDigestDiff bool   `yaml:"upgradeOnDigestDiff"`
	UpgradeScript       string `yaml:"upgradeScript,omitempty"`
}

var (
	date        string
	mailMessage bytes.Buffer
)

func main() {
	// ----- CLI flags -----
	configPath := flag.String("config", "", "Path to configuration file")
	flag.StringVar(configPath, "c", "", "Path to configuration file (shorthand)")
	help := flag.Bool("help", false, "Show help")
	flag.BoolVar(help, "h", false, "Show help (shorthand)")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *configPath == "" {
		log.Fatal("No configuration file passed! Use -c or --config")
		os.Exit(1)
	}

	cfg := loadConfig(*configPath)
	if cfg == nil {
		log.Fatalf("Failed to load config, aborting")
		return
	}

	repos := cfg.Images
	digests := loadDigestState(cfg.DigestStateFilePath)
	digests.prune(digestTrackedRepos(repos))
	date = time.Now().Format("2006-01-02 15:04")
	fmt.Printf("%s: Checking the following repos for newer versions:\n%s", date, lo.Reduce(repos, func(agg string, item *ImageConfig, _ int) string {
		return agg + "  " + item.Name + "\n"
	}, ""))

	for _, repo := range repos {
		current, currentDigest, err := getLocalImage(repo.Name)
		if err != nil {
			fmt.Printf("%s: %s: %v\n", date, repo.Name, err)
			continue
		}

		registry, err := registryClientFor(repo.Registry)
		if err != nil {
			fmt.Printf("%s: %s: %v\n", date, repo.Name, err)
			continue
		}
		latest, latestDigest := registry.latestVersion(repo.Name)

		if current == "" || latest == "" {
			fmt.Printf("%s: %s either current or latest version not defined\n", date, repo.Name)
			continue
		}

		if current == latest {
			if !repo.AlertOnDigestDiff && !repo.UpgradeOnDigestDiff {
				fmt.Printf("%s: %s has latest version installed (%s)\n", date, repo.Name, current)
				continue
			}

			if latestDigest == "" {
				fmt.Printf("%s: %s: could not determine remote digest for %s, skipping digest check\n", date, repo.Name, current)
				continue
			}

			// currentDigest is "" for an image with no RepoDigest of its
			// own, e.g. one built locally FROM repo.Name rather than
			// pulled under that name. Fall back to the digest we recorded
			// for this repo:tag the last time we checked, so a rebuild of
			// the base image can still be detected across runs.
			baselineDigest := currentDigest
			trackingBaseline := false
			if baselineDigest == "" {
				if digests.path == "" {
					fmt.Printf("%s: %s: built locally with no recorded digest, and no digestStateFilePath configured to track one, skipping digest check\n", date, repo.Name)
					continue
				}

				trackingBaseline = true
				baselineDigest = digests.get(repo.Name, current)
				if baselineDigest == "" {
					fmt.Printf("%s: %s: no digest recorded yet for %s, recording the current one as a baseline\n", date, repo.Name, current)
					digests.set(repo.Name, current, latestDigest)
					continue
				}
			}

			handleSameTag(repo, current, baselineDigest, latestDigest)

			if trackingBaseline {
				digests.set(repo.Name, current, latestDigest)
			}
			continue
		}

		if isPatchDiffOnly(current, latest) {
			if repo.UpgradeOnPatchDiff && repo.UpgradeScript != "" {
				fmt.Printf("%s: %s has a newer version available (%s) than currently installed (%s).\n",
					date, repo.Name, latest, current)
				runUpgradeScript(repo, current, latest)
			} else if repo.AlertOnPatchDiff {
				addToAlertMessage(repo.Name, current, latest)
			} else {
				fmt.Printf(
					"%s: %s only has patch diff: current %s, latest %s. Not alerting\n",
					date, repo.Name, current, latest,
				)
			}
		} else {
			addToAlertMessage(repo.Name, current, latest)
		}
	}

	if mailMessage.Len() > 0 {
		fmt.Println("Sending mail now")

		mailer := utils.Mailer(cfg.MailConfigPath)

		if mailer.Error() != "" {
			log.Printf("Mail error: %s\n", mailer.Error())
			return
		}

		message := "Subject: New Docker Versions Available\r\n\r\n" + mailMessage.String()

		if rc := mailer.Send(message); rc != utils.SUCCESS {
			log.Printf("Mail error: %s\n", mailer.Error())
		}
	}
}

// ---------------- helpers ----------------
func addToAlertMessage(repoName, current, latest string) {
	msg := fmt.Sprintf(
		"%s has a newer version available (%s) than currently installed (%s)\n",
		repoName, latest, current,
	)
	fmt.Printf("%s: %s", date, msg)
	mailMessage.WriteString(msg)
}

// handleSameTag is called when the locally installed tag already matches the
// latest tag on the registry, and digest checking is enabled and the digests
// of both images are known. Some images get rebuilt under the same tag (e.g.
// a base-image CVE patch), so the underlying digest can still have changed
// even though the version string did not.
func handleSameTag(repo *ImageConfig, tag, currentDigest, latestDigest string) {
	if currentDigest == latestDigest {
		fmt.Printf("%s: %s has latest tag and checksum installed (%s)\n", date, repo.Name, tag)
		return
	}

	if repo.UpgradeOnDigestDiff && repo.UpgradeScript != "" {
		fmt.Printf("%s: %s has a rebuilt image for tag %s (local digest %s, remote digest %s).\n",
			date, repo.Name, tag, currentDigest, latestDigest)
		runUpgradeScript(repo, tag, tag)
		return
	}

	if repo.AlertOnDigestDiff {
		msg := fmt.Sprintf(
			"%s has a rebuilt image for tag %s (local digest %s, remote digest %s)\n",
			repo.Name, tag, currentDigest, latestDigest,
		)
		fmt.Printf("%s: %s", date, msg)
		mailMessage.WriteString(msg)
	}
}

// runUpgradeScript substitutes from/to into the configured upgrade script and
// runs it, recording the outcome in mailMessage. from and to are equal for a
// digest-only rebuild of the same tag.
func runUpgradeScript(repo *ImageConfig, from, to string) {
	upgradeCmd := strings.ReplaceAll(repo.UpgradeScript, "fromVersionNumber", from)
	upgradeCmd = strings.ReplaceAll(upgradeCmd, "toVersionNumber", to)

	fmt.Printf("%s: %s upgrading using script: %s\n", date, repo.Name, upgradeCmd)

	cmd := exec.Command("bash", "-c", upgradeCmd)
	output, err := cmd.CombinedOutput()

	var messageForMail string
	if err != nil {
		messageForMail = fmt.Sprintf("%s: Error occurred while upgrading %s: %v\n", date, repo.Name, err)
	} else {
		messageForMail = fmt.Sprintf("%s: Successfully upgraded %s from %s to %s\n", date, repo.Name, from, to)
	}
	mailMessage.WriteString(messageForMail)
	fmt.Print(messageForMail)
	fmt.Printf("Output: %s\n", string(output))
}

func isPatchDiffOnly(current, latest string) bool {
	c := parseVersion(current)
	l := parseVersion(latest)

	return c.major == l.major && c.minor == l.minor
}

type version struct {
	major int
	minor int
	patch int
}

func parseVersion(v string) version {
	re := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	m := re.FindStringSubmatch(v)
	if len(m) != 4 {
		return version{}
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])

	return version{
		major: major,
		minor: minor,
		patch: patch,
	}
}

// canonicalLocalRepo maps a registry repo name to the name under which the
// image is tagged locally, for the few repos we build and tag ourselves.
func canonicalLocalRepo(repo string) string {
	repo = strings.TrimPrefix(repo, "library/")

	switch repo {
	case "wordpress":
		return "patklaey/wordpress"
	case "nginx":
		return "patklaey/nginx"
	}

	return repo
}

// getLocalImage returns the non-"latest" tag and manifest digest that the
// local Docker daemon has recorded for repo, read via the Engine API
// (not the `docker images` CLI: filtering it by a repo:tag reference is
// known to report the digest as "<none>" even when the daemon has it,
// see https://github.com/moby/moby/issues/35270).
// digest is "" if the matching image has no recorded RepoDigest, e.g. it
// was built locally rather than pulled.
func getLocalImage(repo string) (tag, digest string, err error) {
	repo = canonicalLocalRepo(repo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli, err := client.New(client.FromEnv)
	if err != nil {
		return "", "", fmt.Errorf("connecting to docker daemon: %w", err)
	}
	defer cli.Close()

	result, err := cli.ImageList(ctx, client.ImageListOptions{
		Filters: client.Filters{}.Add("reference", repo),
	})
	if err != nil {
		return "", "", fmt.Errorf("listing local images for %s: %w", repo, err)
	}

	for _, img := range result.Items {
		for _, repoTag := range img.RepoTags {
			t, ok := strings.CutPrefix(repoTag, repo+":")
			if !ok || t == "latest" {
				continue
			}
			return t, firstDigest(img.RepoDigests, repo), nil
		}
	}

	return "", "", nil
}

// firstDigest returns the digest portion (e.g. "sha256:...") of the first
// entry in repoDigests that belongs to repo. RepoDigests entries have the
// form "repo@sha256:...".
func firstDigest(repoDigests []string, repo string) string {
	for _, rd := range repoDigests {
		if d, ok := strings.CutPrefix(rd, repo+"@"); ok {
			return d
		}
	}
	return ""
}

func isValidVersion(v string) bool {
	re := regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)
	return re.MatchString(v)
}

func sortSematic(versions []string) []string {
	sort.Slice(versions, func(i, j int) bool {
		a := parseVersion(versions[i])
		b := parseVersion(versions[j])

		if a.major != b.major {
			return a.major > b.major // descending
		}
		if a.minor != b.minor {
			return a.minor > b.minor
		}
		return a.patch > b.patch
	})
	return versions
}

func loadConfig(configFile string) *Config {
	var config Config

	yfile, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Println("Could not read config file: ", err)
		return nil
	}

	err = yaml.Unmarshal(yfile, &config)
	if err != nil {
		fmt.Println("Error loading configuration: ", err)
		return nil
	}
	return &config
}
