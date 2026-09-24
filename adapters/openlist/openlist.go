// Package openlist implements an HTTP client for the OpenList API
// (https://github.com/OpenListTeam/OpenList, a.k.a. AList fork), used as the
// network-drive gateway for the "cloud media source" feature.
//
// Why this package exists (设计动机):
// Cloud drives (阿里云盘/百度网盘/etc.) behind OpenList will rate-limit or even ban
// clients that hit their upstream too aggressively (风控). Therefore every request
// leaving this package is:
//   - throttled by a client-level token bucket with ±20% random jitter (see throttle.go),
//   - retried on 429/5xx with exponential backoff (max 3 retries),
//   - guarded by a circuit breaker so a failing upstream degrades into fast-fail
//     instead of hammering it (see breaker.go).
//
// The wire format is OpenList's uniform response envelope:
//
//	{"code": 200, "message": "", "data": ...}
//
// where code != 200 means the request failed (see https://doc.nn.ci/ for the
// official API docs).
package openlist

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Wire protocol constants. OpenList (and AList v3) always return HTTP 200 with an
// application-level code in the envelope body for API-level errors.
const (
	codeOK           = 200
	codeUnauthorized = 401

	endpointLogin = "/api/auth/login"
	endpointList  = "/api/fs/list"
	endpointGet   = "/api/fs/get"
)

var (
	// ErrUnauthorized is returned when the server rejects our credentials/token
	// and we have no way to obtain a fresh one (e.g. explicit-token mode).
	ErrUnauthorized = errors.New("openlist: unauthorized")

	// ErrCircuitOpen is returned fast (without hitting the network) while the
	// circuit breaker is open.
	ErrCircuitOpen = errors.New("openlist: circuit breaker open")

	// errNoCredentials is returned when no token and no username/password are configured.
	errNoCredentials = errors.New("openlist: no token or credentials configured")
)

// APIError is an application-level error reported by OpenList (code != 200).
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openlist: api error %d: %s", e.Code, e.Message)
}

// envelope is the uniform response wrapper of all OpenList API endpoints.
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}
