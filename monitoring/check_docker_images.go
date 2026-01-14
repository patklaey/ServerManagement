package main

import (
	"ServerManagement/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	pageSize = 200
)

type Config struct {
	Images           []string `yaml:"images"`
	AlertOnPatchDiff int      `yaml:"alertOnPatchDiff"`
	MailConfigPath   string   `yaml:"mailConfigPath"`
}

type Image struct {
	Architecture string `json:"architecture"`
}

type Result struct {
	Name   string   `json:"name"`
	Images []*Image `json:"images"`
}

type DockerTagResponse struct {
	Next    string    `json:"next"`
	Results []*Result `json:"results"`
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Dir(exe)

	cfg := loadConfig(filepath.Join(dir, "checkDockerImages.yaml"))
	if cfg == nil {
		log.Fatalf("Failed to load config: %v", err)
		return
	}

	repos := cfg.Images
	alertOnPatchDiff := cfg.AlertOnPatchDiff == 1

	date := time.Now().Format("2006-01-02 15:04")
	fmt.Printf("%s: Checking the following repos for newer versions: %s\n",
		date, strings.Join(repos, " "))

	var mailMessage bytes.Buffer

	for _, repo := range repos {
		current := getCurrentVersion(repo)
		latest := getLatestVersion(repo)

		if current == "" || latest == "" {
			fmt.Printf("%s: %s either current or latest version not defined\n", date, repo)
			continue
		}

		if current == latest {
			fmt.Printf("%s: %s has latest version installed (%s)\n", date, repo, current)
			continue
		}

		if !isPatchDiffOnly(current, latest) || alertOnPatchDiff {
			msg := fmt.Sprintf(
				"%s has a newer version available (%s) than currently installed (%s)\n",
				repo, latest, current,
			)
			fmt.Printf("%s: %s", date, msg)
			mailMessage.WriteString(msg)
		} else {
			fmt.Printf(
				"%s: %s only has patch diff: current %s, latest %s. Not alerting\n",
				date, repo, current, latest,
			)
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

func getCurrentVersion(repo string) string {
	repo = strings.TrimPrefix(repo, "library/")

	switch repo {
	case "wordpress":
		repo = "patklaey/wordpress"
	case "nginx":
		repo = "patklaey/nginx"
	}

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

func getLatestVersion(repo string) string {
	url := fmt.Sprintf(
		"https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=%d",
		repo, pageSize,
	)

	client := &http.Client{}
	var versions []string

	for url != "" {
		resp, err := client.Get(url)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()

		var payload DockerTagResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return ""
		}

		for _, r := range payload.Results {
			if isValidVersion(r.Name) && hasARMImage(r.Images) {
				versions = append(versions, r.Name)
			}
		}

		url = payload.Next
	}

	versions = sortSematic(versions)

	if len(versions) == 0 {
		return ""
	}
	return versions[0]
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

func compareVersion(a, b string) int {
	va := parseVersion(a)
	vb := parseVersion(b)

	if va.major != vb.major {
		return va.major - vb.major
	}
	if va.minor != vb.minor {
		return va.minor - vb.minor
	}
	return va.patch - vb.patch
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
