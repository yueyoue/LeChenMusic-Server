package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/cloudsource"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/request"
)

// cloudSourceHandler exposes diagnostics for the "cloud media source" (网盘媒体源)
// feature: it backs the "test connection" button of the library wizard, so an admin can
// check the OpenList address + remote path *before* saving a library and waiting for a
// scan to find nothing.
//
// It only ever lists a directory (one cheap JSON call) — it deliberately never reads any
// file content, in line with the 风控 rules of docs/网盘媒体源方案设计.md §4.
type cloudSourceHandler struct{}

// cloudSourceTestRequest is what the wizard sends: a raw OpenList address and a remote
// folder path, exactly as typed by the user. The handler composes the storage URI with
// cloudsource.BuildURI, the same code path the library is saved with.
type cloudSourceTestRequest struct {
	URL  string `json:"url"`
	Path string `json:"path"`
}

type cloudSourceEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

type cloudSourceTestResponse struct {
	OK       bool               `json:"ok"`
	Message  string             `json:"message"`
	Endpoint string             `json:"endpoint,omitempty"`
	Entries  []cloudSourceEntry `json:"entries"`
}

// cloudSourceTestMaxEntries keeps the response small: the wizard only shows a preview.
const cloudSourceTestMaxEntries = 10

// cloudSourceTestTimeout bounds how long a flaky gateway can hold the request open.
const cloudSourceTestTimeout = 20 * time.Second

func (h *cloudSourceHandler) test(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, ok := request.UserFrom(ctx)
	if !ok || !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req cloudSourceTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCloudSourceTest(w, cloudSourceTestResponse{Message: "invalid request body"})
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	req.Path = strings.TrimSpace(req.Path)
	if req.URL == "" {
		writeCloudSourceTest(w, cloudSourceTestResponse{Message: "OpenList address is required"})
		return
	}

	uri, err := cloudsource.BuildURI(req.URL, req.Path)
	if err != nil {
		writeCloudSourceTest(w, cloudSourceTestResponse{Message: err.Error()})
		return
	}

	resp := cloudSourceTestResponse{Endpoint: hostOf(req.URL), Entries: []cloudSourceEntry{}}

	ctx, cancel := context.WithTimeout(ctx, cloudSourceTestTimeout)
	defer cancel()

	st, err := storage.For(uri)
	if err != nil {
		resp.Message = err.Error()
		writeCloudSourceTest(w, resp)
		return
	}

	fsys, err := openStorageFS(ctx, st)
	if err != nil {
		log.Warn(ctx, "cloudsource: test connection failed", "uri", uri, err)
		resp.Message = humanCloudError(err)
		writeCloudSourceTest(w, resp)
		return
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		log.Warn(ctx, "cloudsource: test connection could not list the folder", "uri", uri, err)
		resp.Message = humanCloudError(err)
		writeCloudSourceTest(w, resp)
		return
	}

	for i, e := range entries {
		if i >= cloudSourceTestMaxEntries {
			break
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		resp.Entries = append(resp.Entries, cloudSourceEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}

	resp.OK = true
	resp.Message = "ok"
	writeCloudSourceTest(w, resp)
}

// openStorageFS prefers the context-aware constructor so a cancelled/timeout request
// stops hitting the remote gateway.
func openStorageFS(ctx context.Context, st storage.Storage) (storage.MusicFS, error) {
	if cs, ok := st.(storage.ContextualStorage); ok {
		return cs.FSWithContext(ctx)
	}
	return st.FS()
}

// hostOf extracts "host:port" from a raw OpenList address, for the wizard's summary line.
func hostOf(address string) string {
	address = strings.TrimSpace(strings.TrimRight(address, "/"))
	if address == "" {
		return ""
	}
	if i := strings.Index(address, "://"); i >= 0 {
		address = address[i+3:]
	}
	if i := strings.IndexAny(address, "/?"); i >= 0 {
		address = address[:i]
	}
	return address
}

// humanCloudError turns the internal error text into something worth showing in a form.
// Everything else is passed through: the messages from core/cloudsource are already
// specific enough to act on (e.g. "no credentials configured for OpenList endpoint").
func humanCloudError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "remote folder not found"
	case errors.Is(err, context.DeadlineExceeded):
		return "connection timed out"
	default:
		return err.Error()
	}
}

func writeCloudSourceTest(w http.ResponseWriter, resp cloudSourceTestResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error(context.Background(), "Error encoding cloud source test response", err)
	}
}

func (api *Router) addCloudSourceRoute(r chi.Router) {
	h := &cloudSourceHandler{}
	r.Post("/cloudsource/test", h.test)
}
