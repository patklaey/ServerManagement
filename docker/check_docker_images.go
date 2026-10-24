package main

import (
	"ServerManagement/utils"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/samber/lo"
)

const (
	pageSize = 200
)

type Config struct {
	Images         []*ImageConfig `yaml:"images"`
	MailConfigPath string         `yaml:"mailConfigPath"`
}
type ImageConfig struct {
	Name                string `yaml:"name"`
	AlertOnPatchDiff    bool   `yaml:"alertOnPatchDiff"`
	UpgradeOnPatchDiff  bool   `yaml:"upgradeOnPatchDiff"`
	AlertOnDigestDiff   bool   `yaml:"alertOnDigestDiff"`
	UpgradeOnDigestDiff bool   `yaml:"upgradeOnDigestDiff"`
	UpgradeScript       string `yaml:"upgradeScript,omitempty"`
}

type Image struct {
	Architecture string `json:"architecture"`
}

type Result struct {
	Name   string   `json:"name"`
	Images []*Image `json:"images"`
	Digest string   `json:"digest"`
}

type DockerTagResponse struct {
	Next    string    `json:"next"`
	Results []*Result `json:"results"`
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
	date = time.Now().Format("2006-01-02 15:04")
	fmt.Printf("%s: Checking the following repos for newer versions:\n%s", date, lo.Reduce(repos, func(agg string, item *ImageConfig, _ int) string {
		return agg + "  " + item.Name + "\n"
	}, ""))

	for _, repo := range repos {
		current := getCurrentVersion(repo.Name)
		latest, latestDigest := getLatestVersion(repo.Name)

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

			currentDigest := getCurrentDigest(repo.Name, current)
			if currentDigest == "" {
				fmt.Printf("%s: %s: could not determine local digest for %s, skipping digest check\n", date, repo.Name, current)
				continue
			}

			handleSameTag(repo, current, currentDigest, latestDigest)
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
		fmt.Printf("%s: %s has latest version installed (%s)\n", date, repo.Name, tag)
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

func getCurrentVersion(repo string) string {
	repo = canonicalLocalRepo(repo)

	cmd := exec.Command(
		"docker", "images", repo,
		"--format", "{{.Tag}}",
	)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if l != "" && l != "latest" {
			return strings.TrimSpace(l)
		}
	}

	return ""
}

// getCurrentDigest returns the locally recorded manifest digest (RepoDigest)
// for repo:tag, e.g. "sha256:...". It returns "" if the image isn't present
// locally or has no recorded digest (e.g. it was built locally rather than
// pulled).
func getCurrentDigest(repo, tag string) string {
	repo = canonicalLocalRepo(repo)

	cmd := exec.Command(
		"docker", "images", repo+":"+tag,
		"--digests", "--format", "{{.Digest}}",
	)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	digest := strings.TrimSpace(string(out))
	if digest == "<none>" {
		return ""
	}

	return digest
}

// getLatestVersion returns the highest valid semantic version tag available
// for repo that has an ARM image, along with that tag's manifest digest (as
// reported by the registry), so callers can detect a same-tag rebuild
// without a second request.
func getLatestVersion(repo string) (string, string) {
	url := fmt.Sprintf(
		"https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=%d",
		repo, pageSize,
	)

	client := &http.Client{}
	var versions []string
	digestByVersion := make(map[string]string)

	for url != "" {
		resp, err := client.Get(url)
		if err != nil {
			return "", ""
		}

		var payload DockerTagResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil {
			return "", ""
		}

		for _, r := range payload.Results {
			if isValidVersion(r.Name) && hasARMImage(r.Images) {
				versions = append(versions, r.Name)
				digestByVersion[r.Name] = r.Digest
			}
		}

		url = payload.Next
	}

	versions = sortSematic(versions)

	if len(versions) == 0 {
		return "", ""
	}

	latest := versions[0]
	return latest, digestByVersion[latest]
}

func isValidVersion(v string) bool {
	re := regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)
	return re.MatchString(v)
}

func hasARMImage(images []*Image) bool {
	for _, img := range images {
		if strings.Contains(img.Architecture, "arm") {
			return true
		}
	}
	return false
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
