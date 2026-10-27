package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryClientFor(t *testing.T) {
	tests := []struct {
		registry        string
		wantHost        string
		wantFilterByARM bool
		wantErr         bool
	}{
		{"", "registry-1.docker.io", true, false},
		{"dockerhub", "registry-1.docker.io", true, false},
		{"gcr", "", false, false},
		{"quay", "", false, true},
	}

	for _, tt := range tests {
		got, err := registryClientFor(tt.registry)
		if tt.wantErr {
			if err == nil {
				t.Errorf("registryClientFor(%q) expected an error, got none", tt.registry)
			}
			continue
		}
		if err != nil {
			t.Fatalf("registryClientFor(%q) returned unexpected error: %v", tt.registry, err)
		}

		c, ok := got.(v2RegistryClient)
		if !ok {
			t.Fatalf("registryClientFor(%q) = %T, want v2RegistryClient", tt.registry, got)
		}
		if c.host != tt.wantHost || c.filterByARM != tt.wantFilterByARM {
			t.Errorf("registryClientFor(%q) = %+v, want host %q, filterByARM %v", tt.registry, c, tt.wantHost, tt.wantFilterByARM)
		}
	}
}

func TestHasARMPlatform(t *testing.T) {
	tests := []struct {
		platforms []string
		want      bool
	}{
		{[]string{"amd64", "arm64"}, true},
		{[]string{"amd64", "arm"}, true},
		{[]string{"amd64", "386"}, false},
		{nil, false},
	}

	for _, tt := range tests {
		if got := hasARMPlatform(tt.platforms); got != tt.want {
			t.Errorf("hasARMPlatform(%v) = %v, want %v", tt.platforms, got, tt.want)
		}
	}
}

func TestParseManifestResponseReadsDigestAndPlatforms(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Docker-Content-Digest": []string{"sha256:abc"}},
		Body: io.NopCloser(strings.NewReader(`{"manifests":[
			{"platform":{"architecture":"amd64"}},
			{"platform":{"architecture":"arm64"}}
		]}`)),
	}

	info, err := parseManifestResponse(resp)
	if err != nil {
		t.Fatalf("parseManifestResponse returned unexpected error: %v", err)
	}
	if info.digest != "sha256:abc" {
		t.Errorf("digest = %q, want sha256:abc", info.digest)
	}
	if !hasARMPlatform(info.platforms) {
		t.Errorf("platforms = %v, want it to include an arm platform", info.platforms)
	}
}

func TestParseManifestResponseSinglePlatformManifestHasNoPlatforms(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Docker-Content-Digest": []string{"sha256:def"}},
		Body:   io.NopCloser(strings.NewReader(`{"schemaVersion":2,"config":{},"layers":[]}`)),
	}

	info, err := parseManifestResponse(resp)
	if err != nil {
		t.Fatalf("parseManifestResponse returned unexpected error: %v", err)
	}
	if info.digest != "sha256:def" {
		t.Errorf("digest = %q, want sha256:def", info.digest)
	}
	if len(info.platforms) != 0 {
		t.Errorf("platforms = %v, want none for a single-platform manifest", info.platforms)
	}
}

func TestFetchBearerTokenRejectsNonBearerChallenge(t *testing.T) {
	_, err := fetchBearerToken(`Basic realm="example"`)
	if err == nil {
		t.Fatal("expected an error for a non-Bearer challenge, got none")
	}
}

func TestFetchBearerTokenRequiresRealm(t *testing.T) {
	_, err := fetchBearerToken(`Bearer service="gcr.io",scope="repository:x:pull"`)
	if err == nil {
		t.Fatal("expected an error when the challenge has no realm, got none")
	}
}

func TestFetchBearerTokenRequestsFromRealmWithServiceAndScope(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"token":"anonymous-token"}`))
	}))
	defer server.Close()

	challenge := fmt.Sprintf(`Bearer realm="%s",service="gcr.io",scope="repository:my/repo:pull"`, server.URL)

	token, err := fetchBearerToken(challenge)
	if err != nil {
		t.Fatalf("fetchBearerToken returned unexpected error: %v", err)
	}
	if token != "anonymous-token" {
		t.Errorf("fetchBearerToken() = %q, want %q", token, "anonymous-token")
	}
	if !strings.Contains(gotQuery, "service=gcr.io") || !strings.Contains(gotQuery, "scope=repository") {
		t.Errorf("token request query = %q, want it to carry service and scope", gotQuery)
	}
}

func TestFetchBearerTokenFallsBackToAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"legacy-token"}`))
	}))
	defer server.Close()

	challenge := fmt.Sprintf(`Bearer realm="%s",service="gcr.io"`, server.URL)

	token, err := fetchBearerToken(challenge)
	if err != nil {
		t.Fatalf("fetchBearerToken returned unexpected error: %v", err)
	}
	if token != "legacy-token" {
		t.Errorf("fetchBearerToken() = %q, want %q", token, "legacy-token")
	}
}

func TestDoRegistryGetHandlesAnonymousAuthChallenge(t *testing.T) {
	var tokenServerURL string

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s",service="gcr.io",scope="repository:my/repo:pull"`, tokenServerURL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"tags":["1.2.3"]}`))
	}))
	defer registryServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"test-token"}`))
	}))
	defer tokenServer.Close()
	tokenServerURL = tokenServer.URL

	resp, err := doRegistryGet(registryServer.URL, &bearerTokenCache{})
	if err != nil {
		t.Fatalf("doRegistryGet returned unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("doRegistryGet() final status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestBearerTokenCacheReusesTokenAcrossRequests(t *testing.T) {
	var tokenServerURL string
	var tokenRequests int

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s",service="gcr.io",scope="repository:my/repo:pull"`, tokenServerURL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer registryServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests++
		w.Write([]byte(`{"token":"test-token"}`))
	}))
	defer tokenServer.Close()
	tokenServerURL = tokenServer.URL

	auth := &bearerTokenCache{}

	for i := 0; i < 3; i++ {
		resp, err := doRegistryGet(registryServer.URL, auth)
		if err != nil {
			t.Fatalf("request %d: doRegistryGet returned unexpected error: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i, resp.StatusCode, http.StatusOK)
		}
	}

	if tokenRequests != 1 {
		t.Errorf("token endpoint was hit %d times for 3 requests sharing a cache, want 1", tokenRequests)
	}
}
