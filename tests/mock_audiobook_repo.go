package tests

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
)

func CreateMockAudiobookRepo() *MockAudiobookRepo {
	return &MockAudiobookRepo{
		Books:     make(map[string]*model.Audiobook),
		Chapters:  make(map[string][]*model.AudiobookChapter),
		Progress:  make(map[string]*model.AudiobookProgress),
		Bookmarks: make(map[string]*model.AudiobookBookmark),
	}
}

// MockAudiobookRepo is an in-memory model.AudiobookRepository for tests. It is embedded in
// MockDataStore, so scanner/server tests can exercise the audiobook paths without a database.
type MockAudiobookRepo struct {
	model.AudiobookRepository
	Books     map[string]*model.Audiobook          // keyed by book ID
	Chapters  map[string][]*model.AudiobookChapter // keyed by book ID
	Progress  map[string]*model.AudiobookProgress  // keyed by userID + "/" + audiobookID
	Bookmarks map[string]*model.AudiobookBookmark  // keyed by bookmark ID
	Err       bool
}

func (m *MockAudiobookRepo) SetError(err bool) { m.Err = err }

func (m *MockAudiobookRepo) Get(id string) (*model.Audiobook, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	if b, ok := m.Books[id]; ok {
		return b, nil
	}
	return nil, model.ErrNotFound
}

// matchAudiobookFilters understands the handful of Eq filters the scanner and the API use.
// Anything else matches everything, so tests only have to express what they care about.
func matchAudiobookFilters(b *model.Audiobook, f squirrel.Sqlizer) bool {
	eq, ok := f.(squirrel.Eq)
	if !ok {
		return true
	}
	for k, v := range eq {
		want := fmt.Sprint(v)
		switch k {
		case "id":
			if b.ID != want {
				return false
			}
		case "path":
			if b.Path != want {
				return false
			}
		case "library_id":
			if strconv.Itoa(b.LibraryID) != want {
				return false
			}
		case "title":
			if b.Title != want {
				return false
			}
		case "author":
			if b.Author != want {
				return false
			}
		}
	}
	return true
}

func (m *MockAudiobookRepo) GetAll(options ...model.QueryOptions) (model.Audiobooks, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	var opts model.QueryOptions
	if len(options) > 0 {
		opts = options[0]
	}
	var out model.Audiobooks
	for _, b := range m.Books {
		if matchAudiobookFilters(b, opts.Filters) {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

func (m *MockAudiobookRepo) Count(options ...model.QueryOptions) (int64, error) {
	all, err := m.GetAll(options...)
	return int64(len(all)), err
}

func (m *MockAudiobookRepo) Put(book *model.Audiobook) error {
	if m.Err {
		return errors.New("error")
	}
	if book.ID == "" {
		book.ID = id.NewRandom()
	}
	cp := *book
	m.Books[book.ID] = &cp
	return nil
}

func (m *MockAudiobookRepo) Delete(id string) error {
	if m.Err {
		return errors.New("error")
	}
	delete(m.Books, id)
	delete(m.Chapters, id)
	return nil
}

func (m *MockAudiobookRepo) GetChapters(audiobookID string) (model.AudiobookChapters, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	var out model.AudiobookChapters
	for _, c := range m.Chapters[audiobookID] {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChapterNumber < out[j].ChapterNumber })
	return out, nil
}

func (m *MockAudiobookRepo) GetChapter(id string) (*model.AudiobookChapter, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	for _, chapters := range m.Chapters {
		for _, c := range chapters {
			if c.ID == id {
				return c, nil
			}
		}
	}
	return nil, model.ErrNotFound
}

func (m *MockAudiobookRepo) PutChapter(chapter *model.AudiobookChapter) error {
	if m.Err {
		return errors.New("error")
	}
	if chapter.ID == "" {
		chapter.ID = id.NewRandom()
	}
	cp := *chapter
	m.Chapters[chapter.AudiobookID] = append(m.Chapters[chapter.AudiobookID], &cp)
	return nil
}

func (m *MockAudiobookRepo) DeleteChapters(audiobookID string) error {
	if m.Err {
		return errors.New("error")
	}
	delete(m.Chapters, audiobookID)
	return nil
}

func progressKey(userID, audiobookID string) string { return userID + "/" + audiobookID }

func (m *MockAudiobookRepo) GetProgress(userID, audiobookID string) (*model.AudiobookProgress, error) {
	if p, ok := m.Progress[progressKey(userID, audiobookID)]; ok {
		return p, nil
	}
	return nil, model.ErrNotFound
}

func (m *MockAudiobookRepo) SaveProgress(progress *model.AudiobookProgress) error {
	if progress.ID == "" {
		progress.ID = id.NewRandom()
	}
	cp := *progress
	m.Progress[progressKey(progress.UserID, progress.AudiobookID)] = &cp
	return nil
}

func (m *MockAudiobookRepo) GetAllProgress() ([]model.AudiobookProgress, error) {
	var out []model.AudiobookProgress
	for _, p := range m.Progress {
		out = append(out, *p)
	}
	return out, nil
}

func (m *MockAudiobookRepo) GetUserProgress(userID string) ([]model.AudiobookProgress, error) {
	var out []model.AudiobookProgress
	for _, p := range m.Progress {
		if p.UserID == userID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (m *MockAudiobookRepo) GetBookmarks(userID, audiobookID string) ([]model.AudiobookBookmark, error) {
	var out []model.AudiobookBookmark
	for _, b := range m.Bookmarks {
		if b.UserID == userID && b.AudiobookID == audiobookID {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

func (m *MockAudiobookRepo) SaveBookmark(bookmark *model.AudiobookBookmark) error {
	if bookmark.ID == "" {
		bookmark.ID = id.NewRandom()
	}
	cp := *bookmark
	m.Bookmarks[bookmark.ID] = &cp
	return nil
}

func (m *MockAudiobookRepo) DeleteBookmark(id string) error {
	delete(m.Bookmarks, id)
	return nil
}

func (m *MockAudiobookRepo) GetAllBookmarks() ([]model.AudiobookBookmark, error) {
	var out []model.AudiobookBookmark
	for _, b := range m.Bookmarks {
		out = append(out, *b)
	}
	return out, nil
}

func (m *MockAudiobookRepo) Star(userID, audiobookID string) error {
	b, ok := m.Books[audiobookID]
	if !ok {
		return model.ErrNotFound
	}
	b.Starred = "2024-01-01T00:00:00Z"
	return nil
}

func (m *MockAudiobookRepo) Unstar(userID, audiobookID string) error {
	if b, ok := m.Books[audiobookID]; ok {
		b.Starred = ""
	}
	return nil
}

func (m *MockAudiobookRepo) IsStarred(userID, audiobookID string) (bool, error) {
	b, ok := m.Books[audiobookID]
	if !ok {
		return false, nil
	}
	return b.Starred != "", nil
}

func (m *MockAudiobookRepo) GetStarredAt(userID, audiobookID string) (string, error) {
	b, ok := m.Books[audiobookID]
	if !ok {
		return "", model.ErrNotFound
	}
	return b.Starred, nil
}

func (m *MockAudiobookRepo) GetStarred(userID string) (model.Audiobooks, error) {
	var out model.Audiobooks
	for _, b := range m.Books {
		if b.Starred != "" {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}
