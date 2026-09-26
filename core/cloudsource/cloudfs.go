package cloudsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/adapters/openlist"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/storage/local"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/metadata"
)

// maxListPages bounds the /api/fs/list pagination loop, so a driver that never stops
// returning entries can't spin forever.
const maxListPages = 10000

// remotePathOf maps an fs.FS-relative path ("" or "." = the library root) to the
// absolute path expected by the OpenList API.
func remotePathOf(root, name string) string {
	p := root
	if name != "" && name != "." {
		p = path.Join(root, name)
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return "/"
	}
	return "/" + p
}

// cloudFS implements storage.MusicFS on top of an OpenList gateway.
//
// It is a lazy, listing-only view of the remote folder: enumerating directories only
// costs one cheap JSON call (cached), and file contents are read through remoteFile,
// which fetches bounded byte ranges on demand.
type cloudFS struct {
	ctx  context.Context
	ep   *Endpoint
	root string // remote path below the drive root, no leading/trailing slash

	rootOnce sync.Once
	rootInfo *cloudInfo
	rootErr  error
}

// Open implements fs.FS. Directories yield a ReadDirFile, files yield a lazy,
// seekable reader (remoteFile).
func (f *cloudFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	inf, err := f.Stat(name)
	if err != nil {
		return nil, err
	}
	ci, ok := inf.(*cloudInfo)
	if !ok {
		ci = infoToCloudInfo(inf)
	}
	if ci.IsDir() {
		return &cloudDir{fsys: f, name: name, info: ci}, nil
	}
	return newRemoteFile(f.ctx, f.ep, f.root, name, ci), nil
}

// Stat implements fs.StatFS. Info for anything below the root comes from the (cached)
// listing of its parent, so it never costs an extra upstream call during a scan.
func (f *cloudFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		return f.rootStat()
	}
	parent := path.Dir(name)
	base := path.Base(name)
	entries, err := f.listDir(parent)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	for _, e := range entries {
		if e.Name == base {
			return entryToInfo(e), nil
		}
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (f *cloudFS) rootStat() (fs.FileInfo, error) {
	f.rootOnce.Do(func() {
		remote := remotePathOf(f.root, ".")
		info, err := f.ep.client.Get(f.ctx, remote)
		switch {
		case err == nil:
			f.rootInfo = entryToInfo(openlist.Entry{
				Name:     path.Base(f.root),
				Size:     info.Size,
				IsDir:    true,
				Modified: info.Modified,
				Created:  info.Created,
			})
		case errors.Is(err, openlist.ErrCircuitOpen):
			f.rootErr = err
		default:
			// /api/fs/get on a directory is not supported by every driver. The root is a
			// directory no matter what it says about its mtime, so only give up when we
			// can't even reach the gateway.
			log.Debug(f.ctx, "cloudsource: could not stat root dir, assuming directory", "path", remote, err)
			f.rootInfo = &cloudInfo{name: path.Base(f.root), dir: true}
		}
	})
	if f.rootErr != nil {
		return nil, &fs.PathError{Op: "stat", Path: ".", Err: f.rootErr}
	}
	return f.rootInfo, nil
}

// ReadDir implements fs.ReadDirFS (sorted by name, as required by io/fs).
func (f *cloudFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	entries, err := f.listDir(name)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	out := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &cloudDirEntry{entryToInfo(e)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (f *cloudFS) listDir(name string) ([]openlist.Entry, error) {
	return f.ep.listDir(f.ctx, remotePathOf(f.root, name))
}

// DirectURL implements storage.DirectLinkProvider. It honours the DisableRedirect
// switch: when set, the operator wants every playback relayed through this server and
// callers get an error here so they fall back to the 中转流.
func (f *cloudFS) DirectURL(ctx context.Context, name string) (string, time.Time, error) {
	if !f.ep.supportsRedirect() {
		return "", time.Time{}, errRedirectDisabled
	}
	if ctx == nil {
		ctx = f.ctx
	}
	return f.ep.directURL(ctx, remotePathOf(f.root, name))
}

// ReadTags implements storage.MusicFS. It reuses the very same tag parsers as the local
// storage (go-taglib), feeding them a *bounded* reader: only the head/tail byte ranges
// of the file are ever fetched (see remoteFile), never the whole file.
//
// When a file can't be parsed at all (unsupported format, read budget exceeded, ...), it
// falls back to the 艺人/专辑/曲目 folder convention from the design doc, so a cloud
// library always builds.
func (f *cloudFS) ReadTags(paths ...string) (map[string]metadata.Info, error) {
	if len(paths) == 0 {
		return map[string]metadata.Info{}, nil
	}

	out := make(map[string]metadata.Info, len(paths))

	if filenameOnly() {
		for _, p := range paths {
			info := filenameInfo(p)
			fi, err := fs.Stat(f, p)
			if err != nil {
				return nil, err
			}
			info.FileInfo = cloudFileInfo{fi}
			out[p] = info
		}
		return out, nil
	}

	ex, ok := local.ExtractorFor(extractorID(), &budgetFS{fsys: f, budget: f.ep.maxTagReadBytes}, "")
	if !ok {
		return nil, fmt.Errorf("cloudsource: no metadata extractor registered (Scanner.Extractor=%q)", extractorID())
	}

	parsed, err := ex.Parse(paths...)
	if err != nil {
		return nil, err
	}

	for _, p := range paths {
		info, ok := parsed[p]
		if !ok || len(info.Tags) == 0 {
			info = filenameInfo(p)
		}
		if info.FileInfo == nil {
			fi, err := fs.Stat(f, p)
			if err != nil {
				return nil, err
			}
			info.FileInfo = cloudFileInfo{fi}
		}
		out[p] = info
	}
	return out, nil
}

// extractorID returns the configured tag extractor, defaulting to "taglib".
func extractorID() string {
	if id := conf.Server.Scanner.Extractor; id != "" {
		return id
	}
	return consts.DefaultScannerExtractor
}

// filenameOnly reports whether cloud libraries must be built without reading any file
// content at all (see OpenList.<name>.TagMode in the design doc).
func filenameOnly() bool {
	for _, opts := range conf.Server.OpenList {
		if strings.EqualFold(opts.TagMode, "filename") {
			return true
		}
	}
	return false
}

// budgetFS wraps a MusicFS so that every file it opens has a hard cap on how many bytes
// may be pulled from the network. It exists so a tag parser can never turn a "read the
// header" into a full-file download (a 下载器特征 we must not have, see design doc §4).
type budgetFS struct {
	fsys   *cloudFS
	budget int64
}

func (b *budgetFS) Open(name string) (fs.File, error) {
	f, err := b.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	if rf, ok := f.(*remoteFile); ok {
		rf.budget = b.budget
	}
	return f, nil
}

// ---- dir entries / file info ----

type cloudInfo struct {
	name    string
	size    int64
	dir     bool
	modTime time.Time
	created time.Time
}

func (i *cloudInfo) Name() string { return i.name }
func (i *cloudInfo) Size() int64  { return i.size }
func (i *cloudInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i *cloudInfo) ModTime() time.Time { return i.modTime }
func (i *cloudInfo) IsDir() bool        { return i.dir }
func (i *cloudInfo) Sys() any           { return nil }
func (i *cloudInfo) BirthTime() time.Time {
	if i.created.IsZero() {
		return i.modTime
	}
	return i.created
}

func entryToInfo(e openlist.Entry) *cloudInfo {
	return &cloudInfo{
		name:    e.Name,
		size:    e.Size,
		dir:     e.IsDir,
		modTime: e.Modified,
		created: e.Created,
	}
}

func infoToCloudInfo(inf fs.FileInfo) *cloudInfo {
	if ci, ok := inf.(*cloudInfo); ok {
		return ci
	}
	return &cloudInfo{name: inf.Name(), size: inf.Size(), dir: inf.IsDir(), modTime: inf.ModTime()}
}

// cloudFileInfo adapts fs.FileInfo to metadata.FileInfo (which also wants BirthTime).
type cloudFileInfo struct {
	fs.FileInfo
}

func (c cloudFileInfo) BirthTime() time.Time {
	if bt, ok := c.FileInfo.(interface{ BirthTime() time.Time }); ok {
		return bt.BirthTime()
	}
	return c.FileInfo.ModTime()
}

type cloudDirEntry struct {
	info *cloudInfo
}

func (d *cloudDirEntry) Name() string               { return d.info.Name() }
func (d *cloudDirEntry) IsDir() bool                { return d.info.IsDir() }
func (d *cloudDirEntry) Type() fs.FileMode          { return d.info.Mode().Type() }
func (d *cloudDirEntry) Info() (fs.FileInfo, error) { return d.info, nil }

// cloudDir is the fs.ReadDirFile returned for directories.
type cloudDir struct {
	fsys *cloudFS
	name string
	info *cloudInfo
	pos  int
	read []fs.DirEntry
	done bool
}

func (d *cloudDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *cloudDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}
func (d *cloudDir) Close() error { return nil }

func (d *cloudDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.read == nil {
		entries, err := d.fsys.ReadDir(d.name)
		if err != nil {
			return nil, err
		}
		d.read = entries
	}
	if d.pos >= len(d.read) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	end := len(d.read)
	if n > 0 && d.pos+n < end {
		end = d.pos + n
	}
	out := d.read[d.pos:end]
	d.pos = end
	return out, nil
}

// ---- endpoint level helpers ----

// listDir lists a remote directory, paginating and strongly caching the result.
// Pagination stops as soon as the gateway-reported total is reached, so a listing costs
// exactly ceil(total/pageSize) requests and never a speculative extra one.
func (e *Endpoint) listDir(ctx context.Context, remote string) ([]openlist.Entry, error) {
	if entries, ok := e.dirs.Get(remote); ok {
		return entries, nil
	}

	var all []openlist.Entry
	prevFirst := ""
	for page := 1; page <= maxListPages; page++ {
		entries, total, err := e.client.ListPaged(ctx, remote, page)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			break
		}
		if page > 1 && entries[0].Name == prevFirst {
			// The driver ignores pagination and returns the same page again.
			break
		}
		prevFirst = entries[0].Name
		all = append(all, entries...)
		if total > 0 && len(all) >= total {
			break
		}
	}
	e.dirs.Set(remote, all)
	return all, nil
}

// directURL resolves a fresh, signed download link for a remote file. Links expire in
// hours at most, so one is resolved per playback and never persisted (design doc §2.2).
func (e *Endpoint) directURL(ctx context.Context, remote string) (string, time.Time, error) {
	info, err := e.client.Get(ctx, remote)
	if err != nil {
		return "", time.Time{}, err
	}
	if info.IsDir {
		return "", time.Time{}, fmt.Errorf("cloudsource: %s is a directory", remote)
	}
	if info.RawURL == "" {
		return "", time.Time{}, fmt.Errorf("cloudsource: gateway returned no direct link for %s", remote)
	}
	return info.RawURL, info.ExpiresAt, nil
}
