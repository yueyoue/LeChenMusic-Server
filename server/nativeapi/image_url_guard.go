package nativeapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/navidrome/navidrome/log"
)

// Remote-image fetches (cover-by-URL, narrator-avatar-by-URL and the cover_url proxy)
// take user-supplied URLs. Without validation this is a textbook SSRF: anyone could make
// the server GET internal addresses (http://192.168.x.x, http://169.254.169.254/…), probe
// the LAN, or use exotic schemes to reach local resources (评审 §4 P0-2).
//
// Guardrails enforced here:
//   - only http/https schemes;
//   - the host must resolve to a public address (no loopback / private / link-local /
//     multicast / unspecified);
//   - redirects are re-validated hop by hop (a public URL must not redirect into the LAN),
//     bounded by maxRedirects;
//   - response size is capped by MaxImageUploadSize and the request times out.

// lookupImageHost resolves a host for validation. It is a seam so unit tests can run
// without DNS and can simulate internal resolutions.
var lookupImageHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

const (
	// maxRedirects bounds redirect following for remote image fetches.
	maxRedirects = 5
	// remoteImageTimeout bounds a single remote image request.
	remoteImageTimeout = 30 * time.Second
)

// validateImageURL reports whether rawURL is acceptable to fetch as a remote image.
func validateImageURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid image URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("image URL scheme %q is not allowed (only http/https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("image URL has no host")
	}
	addrs, err := lookupImageHost(ctx, host)
	if err != nil {
		return fmt.Errorf("cannot resolve image URL host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("cannot resolve image URL host %q", host)
	}
	for _, a := range addrs {
		if isForbiddenImageIP(a.IP) {
			return fmt.Errorf("image URL host %q resolves to a non-public address, refusing to fetch", host)
		}
	}
	return nil
}

// isForbiddenImageIP covers the address ranges a server-side fetch must never hit on
// behalf of a user: loopback, RFC1918 private, link-local (incl. cloud metadata
// 169.254.169.254), multicast and the unspecified address.
func isForbiddenImageIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}

// remoteImageClient returns an HTTP client that re-validates every redirect hop.
func remoteImageClient() *http.Client {
	return &http.Client{
		Timeout: remoteImageTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return validateImageURL(req.Context(), req.URL.String())
		},
	}
}

// fetchRemoteImage downloads an image from a user-supplied URL with the SSRF guardrails
// above. It returns the body (already capped) and the declared content type. The raw URL
// is never logged — it can carry signed query parameters.
func fetchRemoteImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	if err := validateImageURL(ctx, rawURL); err != nil {
		if u, parseErr := url.Parse(rawURL); parseErr == nil {
			log.Warn(ctx, "Refusing remote image URL", "scheme", u.Scheme, "host", u.Hostname(), err)
		}
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "LeChenMusic-Server/1.0")

	resp, err := remoteImageClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("image URL returned HTTP %s", resp.Status)
	}

	limit := maxImageUploadSize()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > limit {
		return nil, "", fmt.Errorf("image exceeds the maximum upload size (%d bytes)", limit)
	}
	return body, resp.Header.Get("Content-Type"), nil
}
