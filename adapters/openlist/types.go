package openlist

import (
	"encoding/json"
	"fmt"
	"time"
)

// Entry is one item in a directory listing (POST /api/fs/list → data.content[]).
// Field names mirror the OpenList "ObjResp" JSON object.
type Entry struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Modified time.Time `json:"modified"`
	Created  time.Time `json:"created"`
	Thumb    string    `json:"thumb"`
	Sign     string    `json:"sign"`
	Type     int       `json:"type"`
}

// FileInfo is the detail of a single object (POST /api/fs/get → data).
type FileInfo struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Modified time.Time `json:"modified"`
	Created  time.Time `json:"created"`
	Thumb    string    `json:"thumb"`
	Sign     string    `json:"sign"`

	// RawURL is the direct download URL for the file. For some storage drivers it
	// is a signed URL with a limited lifetime — see ExpiresAt.
	RawURL string `json:"raw_url"`

	// ExpiresAt is the expiry time of RawURL when the response carries one
	// (some drivers/versions expose "expired"/"expires"/"expires_at", as either a
	// unix timestamp or an RFC3339 string). Zero value = no expiry info given.
	ExpiresAt time.Time `json:"-"`
}

// UnmarshalJSON decodes the fs/get payload. The expiry fields are not stable
// across OpenList versions/drivers, so they are probed opportunistically instead
// of being hard-typed into the struct.
func (f *FileInfo) UnmarshalJSON(data []byte) error {
	type plain FileInfo // avoid recursion
	var p plain
	// Decode known fields leniently first.
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("openlist: decode FileInfo: %w", err)
	}
	*f = FileInfo(p)

	// Probe for expiry hints in any of the known spellings.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("openlist: decode FileInfo extras: %w", err)
	}
	for _, key := range []string{"expires_at", "expires", "expired", "expire"} {
		v, ok := raw[key]
		if !ok {
			continue
		}
		if t, ok := parseExpiry(v); ok {
			f.ExpiresAt = t
			break
		}
	}
	return nil
}

// parseExpiry accepts unix seconds ("1700000000"), float seconds
// ("1700000000.123") or RFC3339 strings ("2026-01-01T00:00:00Z").
func parseExpiry(v json.RawMessage) (time.Time, bool) {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
		var n int64
		if _, err := fmt.Sscan(s, &n); err == nil && n > 0 {
			return time.Unix(n, 0), true
		}
		return time.Time{}, false
	}
	var n float64
	if err := json.Unmarshal(v, &n); err == nil && n > 0 {
		return time.Unix(int64(n), 0), true
	}
	return time.Time{}, false
}

// loginRequest is the body of POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse wraps the `data` field of the login response. OpenList returns
// {"token": "..."}; some AList v3 builds return the bare token as a JSON string,
// so both shapes are accepted.
type loginResponse struct {
	Token string
}

func (l *loginResponse) UnmarshalJSON(data []byte) error {
	var obj struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &obj); err == nil && obj.Token != "" {
		l.Token = obj.Token
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("openlist: decode login response: %w", err)
	}
	l.Token = s
	return nil
}

// listRequest is the body of POST /api/fs/list. `refresh` forces OpenList to
// bypass its own directory cache — we keep it false to avoid extra upstream hits.
type listRequest struct {
	Path    string `json:"path"`
	Page    int    `json:"page"`
	Refresh bool   `json:"refresh"`
}

// listResponse is the `data` of POST /api/fs/list. `total`/`page` are kept for
// callers that want to implement smarter pagination than "stop on empty page".
type listResponse struct {
	Content []Entry `json:"content"`
	Total   int     `json:"total"`
}

// getRequest is the body of POST /api/fs/get.
type getRequest struct {
	Path string `json:"path"`
}
