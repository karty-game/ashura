//go:build !js

package main

import (
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGalleryWorksBehindPathPrefix(t *testing.T) {
	t.Parallel()
	dist := t.TempDir()

	const hostileName = `<script>alert("name")</script>`

	samples := []gallerySample{
		{Name: "Basic scene", Path: "samples/basic-scene/"},
		{Name: hostileName, Path: "samples/other/"},
	}

	writeWebFixtures(t, dist, map[string]string{
		"wasm_exec.js":                 "// shared runtime",
		"samples/basic-scene/app.wasm": "basic wasm fixture",
		"samples/other/app.wasm":       "other wasm fixture",
	})

	for _, sample := range samples {
		directory := filepath.Join(dist, filepath.FromSlash(sample.Path))
		if err := writeSamplePage(directory, "../../wasm_exec.js", sample.Name); err != nil {
			t.Fatal(err)
		}
	}

	if err := writeGallery(dist, samples); err != nil {
		t.Fatal(err)
	}

	const prefix = "/proxy/4321/"

	mux := http.NewServeMux()
	mux.Handle(prefix, http.StripPrefix(prefix, http.FileServer(http.Dir(dist))))
	server := httptest.NewServer(mux)

	defer server.Close()

	base, err := url.Parse(server.URL + prefix)
	if err != nil {
		t.Fatal(err)
	}

	body := fetchWebFixture(t, server.Client(), base)
	if strings.Contains(body, hostileName) || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("sample name was not escaped as page text")
	}

	links := regexp.MustCompile(`href="([^"]+)"`).FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)

	for _, link := range links {
		reference, parseErr := url.Parse(html.UnescapeString(link[1]))
		if parseErr != nil || reference.Path == "" {
			continue
		}

		resolved := base.ResolveReference(reference)
		if !strings.HasPrefix(resolved.Path, prefix) {
			t.Fatalf("gallery link escaped preview prefix: %s", resolved)
		}

		if !strings.HasPrefix(reference.Path, "./samples/") && !strings.HasPrefix(reference.Path, "samples/") {
			continue
		}

		page := fetchWebFixture(t, server.Client(), resolved)
		if !strings.Contains(page, " — Ashura</title>") || strings.Contains(page, hostileName) {
			t.Fatal("standalone sample title missing or unescaped")
		}

		assertSampleResources(t, server.Client(), resolved, page)
		seen[resolved.Path] = true
	}

	if len(seen) != len(samples) {
		t.Fatalf("linked sample pages = %d, want %d", len(seen), len(samples))
	}
}

func assertSampleResources(t *testing.T, client *http.Client, base *url.URL, page string) {
	t.Helper()

	resources := []struct{ path, marker string }{
		{"../../wasm_exec.js", "shared runtime"},
		{"./app.wasm", "wasm fixture"},
	}
	for _, resource := range resources {
		if !strings.Contains(page, resource.path) {
			t.Fatalf("sample page does not reference %s", resource.path)
		}

		reference, err := url.Parse(resource.path)
		if err != nil {
			t.Fatal(err)
		}

		contents := fetchWebFixture(t, client, base.ResolveReference(reference))
		if !strings.Contains(contents, resource.marker) {
			t.Fatalf("unexpected resource at %s", resource.path)
		}
	}
}

func fetchWebFixture(t *testing.T, client *http.Client, address *url.URL) string {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address.String(), nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %s", address, response.StatusCode, body)
	}

	return string(body)
}
