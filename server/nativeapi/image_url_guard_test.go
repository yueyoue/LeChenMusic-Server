package nativeapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/tests"
)

// Regression tests for 评审 §4 P0-2: user-supplied image URLs must not be usable to make
// the server fetch internal/loopback/cloud-metadata addresses (SSRF).

func stubImageHost(t *testing.T, fn func(host string) []net.IPAddr) {
	t.Helper()
	orig := lookupImageHost
	lookupImageHost = func(_ context.Context, host string) ([]net.IPAddr, error) {
		return fn(host), nil
	}
	t.Cleanup(func() { lookupImageHost = orig })
}

func TestValidateImageURLRejectsNonHTTPSchemes(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"ftp://example.com/a.png",
		"gopher://example.com/a.png",
		"data:image/png;base64,AAAA",
		"javascript:alert(1)",
	} {
		if err := validateImageURL(context.Background(), raw); err == nil {
			t.Errorf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateImageURLRejectsInternalAddresses(t *testing.T) {
	// Literal IPs need no DNS: the guard must reject them outright.
	for _, raw := range []string{
		"http://127.0.0.1/secret.png",
		"http://[::1]/secret.png",
		"http://10.0.0.5/secret.png",
		"http://192.168.1.10/secret.png",
		"http://172.16.0.1/secret.png",
		"http://169.254.169.254/latest/meta-data/",
		"http://[fe80::1]/secret.png",
		"http://[fc00::1]/secret.png",
		"http://0.0.0.0/secret.png",
	} {
		if err := validateImageURL(context.Background(), raw); err == nil {
			t.Errorf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateImageURLAcceptsPublicHost(t *testing.T) {
	stubImageHost(t, func(string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}
	})
	if err := validateImageURL(context.Background(), "https://example.com/a.png"); err != nil {
		t.Fatalf("public host must be allowed, got %v", err)
	}
}

func TestValidateImageURLRejectsHostResolvingInternally(t *testing.T) {
	stubImageHost(t, func(string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("10.1.2.3")}}
	})
	if err := validateImageURL(context.Background(), "https://evil.example.com/a.png"); err == nil {
		t.Fatal("host resolving to a private address must be rejected")
	}
}

func TestFetchRemoteImageDownloadsBody(t *testing.T) {
	tests.Init(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	// The httptest server is on loopback; stub the resolution to a public IP so only the
	// download path is exercised here (the address guard has its own tests above).
	stubImageHost(t, func(string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}
	})

	data, contentType, err := fetchRemoteImage(context.Background(), srv.URL+"/a.png")
	if err != nil {
		t.Fatalf("expected download to succeed, got %v", err)
	}
	if string(data) != "png-bytes" || contentType != "image/png" {
		t.Fatalf("unexpected response: data=%q type=%q", data, contentType)
	}
}

func TestFetchRemoteImageBlocksRedirectToInternal(t *testing.T) {
	tests.Init(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://10.0.0.1/secret.png")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	stubImageHost(t, func(host string) []net.IPAddr {
		if strings.HasPrefix(host, "10.") {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}
	})

	if _, _, err := fetchRemoteImage(context.Background(), srv.URL+"/a.png"); err == nil {
		t.Fatal("redirect into an internal address must be rejected")
	}
}

func TestFetchRemoteImageRejectsOversizedImage(t *testing.T) {
	tests.Init(t, false)
	conf.Server.MaxImageUploadSize = "50B"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()

	stubImageHost(t, func(string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}
	})

	if _, _, err := fetchRemoteImage(context.Background(), srv.URL+"/big.png"); err == nil {
		t.Fatal("oversized image must be rejected")
	}
}

func TestFetchRemoteImageRejectsNonOKStatus(t *testing.T) {
	tests.Init(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	stubImageHost(t, func(string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}
	})

	if _, _, err := fetchRemoteImage(context.Background(), srv.URL+"/a.png"); err == nil {
		t.Fatal("non-200 responses must be rejected")
	}
}
