package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// registryClient looks up the latest available tag (and, if known, its
// manifest digest) for a repo hosted on some container registry.
type registryClient interface {
	latestVersion(repo string) (tag, digest string)
}

// v2RegistryClient looks up tags via the standard Docker Registry v2
// protocol (https://distribution.github.io/distribution/spec/api/), which
// every registry used here speaks, including Docker Hub itself at
// registry-1.docker.io.
//
// The v2 protocol has no notion of "does this tag have an ARM image" the
// way Docker Hub's proprietary hub.docker.com convenience API does, so when
// filterByARM is set, each candidate tag's manifest (list) is fetched,
// starting from the highest version, until one with an arm/arm64 platform
// is found.
type v2RegistryClient struct {
	// host is the registry host to query. If empty, it is instead parsed
	// from the leading "host/" segment of repo (e.g. for "gcr.io/...").
	host string
	// filterByARM requires a tag's manifest list to include an arm/arm64
	// platform before it is accepted as "latest".
	filterByARM bool
}

// registryClientFor returns the registryClient for the given config value,
// defaulting to Docker Hub when registry is empty.
func registryClientFor(registry string) (registryClient, error) {
	switch registry {
	case "", "dockerhub":
		return v2RegistryClient{host: "registry-1.docker.io", filterByARM: true}, nil
	case "gcr":
		return v2RegistryClient{filterByARM: true}, nil
	default:
		return nil, fmt.Errorf("unknown registry %q", registry)
	}
}

func (c v2RegistryClient) latestVersion(repo string) (string, string) {
	host, path := c.host, repo

	if host == "" {
		h, p, ok := strings.Cut(repo, "/")
		if !ok {
			fmt.Printf("%s: %s: expected a repo of the form host/path, e.g. gcr.io/project/image\n", date, repo)
			return "", ""
		}
		host, path = h, p
	} else if !strings.Contains(path, "/") {
		// The v2 API requires the "library/" namespace for official
		// images; Docker Hub's hub.docker.com convenience API (unlike the
		// v2 API) infers this automatically for bare names like "nginx".
		path = "library/" + path
	}

	tags, err := fetchTags(host, path)
	if err != nil {
		fmt.Printf("%s: %s: fetching tags: %v\n", date, repo, err)
		return "", ""
	}

	var versions []string
	for _, tag := range tags {
		if isValidVersion(tag) {
			versions = append(versions, tag)
		}
	}
	versions = sortSematic(versions)

	for _, tag := range versions {
		info, err := fetchManifestInfo(host, path, tag)
		if err != nil {
			fmt.Printf("%s: %s: fetching manifest for %s: %v\n", date, repo, tag, err)
			continue
		}
		if !c.filterByARM || hasARMPlatform(info.platforms) {
			return tag, info.digest
		}
	}

	return "", ""
}

// fetchTags returns the tags reported by GET /v2/<path>/tags/list on host.
func fetchTags(host, path string) ([]string, error) {
	tagsURL := fmt.Sprintf("https://%s/v2/%s/tags/list", host, path)

	resp, err := doRegistryGet(tagsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var payload struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return payload.Tags, nil
}

// manifestInfo is what we need out of GET /v2/<path>/manifests/<tag>: the
// manifest's own content digest (from the Docker-Content-Digest response
// header, which matches the RepoDigest Docker records locally when the
// image is pulled) and, if it is a manifest list/OCI index, the platforms
// it covers.
type manifestInfo struct {
	digest    string
	platforms []string
}

// fetchManifestInfo requests tag's manifest from host, accepting both
// manifest lists/OCI indexes and single-platform manifests.
func fetchManifestInfo(host, path, tag string) (manifestInfo, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", host, path, tag)

	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return manifestInfo{}, err
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))

	resp, err := doRegistryRequest(req)
	if err != nil {
		return manifestInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return manifestInfo{}, fmt.Errorf("unexpected status %s", resp.Status)
	}

	return parseManifestResponse(resp)
}

// parseManifestResponse reads a manifest (list) response into a
// manifestInfo. A single-platform manifest has no "manifests" list, so it
// decodes to a zero-value body and info.platforms stays empty; that's fine,
// filterByARM treats "unknown platforms" the same as "no ARM found".
func parseManifestResponse(resp *http.Response) (manifestInfo, error) {
	info := manifestInfo{digest: resp.Header.Get("Docker-Content-Digest")}

	var body struct {
		Manifests []struct {
			Platform struct {
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return manifestInfo{}, fmt.Errorf("decoding response: %w", err)
	}
	for _, m := range body.Manifests {
		info.platforms = append(info.platforms, m.Platform.Architecture)
	}

	return info, nil
}

func hasARMPlatform(platforms []string) bool {
	for _, arch := range platforms {
		if strings.Contains(arch, "arm") {
			return true
		}
	}
	return false
}

var registryHTTPClient = &http.Client{Timeout: 15 * time.Second}

// doRegistryGet is a GET against url with no extra headers; see
// doRegistryRequest.
func doRegistryGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return doRegistryRequest(req)
}

// doRegistryRequest performs req against a Docker Registry v2 endpoint,
// transparently handling the anonymous bearer-token challenge that v2
// registries (Docker Hub, GCR, ...) require even for public images.
func doRegistryRequest(req *http.Request) (*http.Response, error) {
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("Www-Authenticate")
	resp.Body.Close()

	token, err := fetchBearerToken(challenge)
	if err != nil {
		return nil, fmt.Errorf("authenticating: %w", err)
	}

	retry := req.Clone(req.Context())
	retry.Header.Set("Authorization", "Bearer "+token)

	return registryHTTPClient.Do(retry)
}

var bearerChallengeParamRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// fetchBearerToken requests an (anonymous, for public images) token from the
// realm named in a WWW-Authenticate: Bearer challenge header, per the Docker
// Registry v2 token authentication spec.
func fetchBearerToken(challenge string) (string, error) {
	if !strings.HasPrefix(challenge, "Bearer ") {
		return "", fmt.Errorf("unsupported auth challenge %q", challenge)
	}

	params := make(map[string]string)
	for _, m := range bearerChallengeParamRe.FindAllStringSubmatch(challenge, -1) {
		params[m[1]] = m[2]
	}

	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("missing realm in challenge %q", challenge)
	}

	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("invalid realm %q: %w", realm, err)
	}

	q := tokenURL.Query()
	if service := params["service"]; service != "" {
		q.Set("service", service)
	}
	if scope := params["scope"]; scope != "" {
		q.Set("scope", scope)
	}
	tokenURL.RawQuery = q.Encode()

	resp, err := registryHTTPClient.Get(tokenURL.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %s", resp.Status)
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	if payload.Token != "" {
		return payload.Token, nil
	}
	return payload.AccessToken, nil
}
