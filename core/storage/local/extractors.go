package local

import (
	"io/fs"
	"sync"

	"github.com/navidrome/navidrome/model/metadata"
)

// Extractor is an interface that defines the methods that a tag/metadata extractor must implement
type Extractor interface {
	Parse(files ...string) (map[string]metadata.Info, error)
	Version() string
}

type extractorConstructor func(fs.FS, string) Extractor

var (
	extractors = map[string]extractorConstructor{}
	lock       sync.RWMutex
)

// RegisterExtractor registers a new extractor, so it can be used by the local storage. The one to be used is
// defined with the configuration option Scanner.Extractor.
func RegisterExtractor(id string, f extractorConstructor) {
	lock.Lock()
	defer lock.Unlock()
	extractors[id] = f
}

// ExtractorFor builds an extractor by its registered id, bound to the given filesystem.
// It lets other storage backends (e.g. cloud sources) reuse the very same tag parsers as
// the local storage, instead of reimplementing them. Returns false when the id is unknown
// (e.g. its registering package was not linked into the binary).
func ExtractorFor(id string, fsys fs.FS, baseDir string) (Extractor, bool) {
	lock.RLock()
	defer lock.RUnlock()
	c, ok := extractors[id]
	if !ok || c == nil {
		return nil, false
	}
	return c(fsys, baseDir), true
}

// ExtractorIDs returns the ids of all registered extractors, for diagnostics.
func ExtractorIDs() []string {
	lock.RLock()
	defer lock.RUnlock()
	ids := make([]string, 0, len(extractors))
	for id := range extractors {
		ids = append(ids, id)
	}
	return ids
}
