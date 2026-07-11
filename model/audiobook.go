package model

import (
	"time"
)

// [LeChenMusic-START:audiobook] Audiobook data models

type Audiobook struct {
	ID            string    `structs:"id"            json:"id"            db:"id"`
	Title         string    `structs:"title"         json:"title"         db:"title"`
	Author        string    `structs:"author"        json:"author"        db:"author"`
	Narrator      string    `structs:"narrator"      json:"narrator"      db:"narrator"`
	Description   string    `structs:"description"   json:"description"   db:"description"`
	Genre         string    `structs:"genre"         json:"genre"         db:"genre"`
	Year          int       `structs:"year"          json:"year"          db:"year"`
	CoverPath     string    `structs:"cover_path"    json:"coverPath"     db:"cover_path"`
	TotalDuration int       `structs:"total_duration" json:"totalDuration" db:"total_duration"`
	ChapterCount  int       `structs:"chapter_count" json:"chapterCount"  db:"chapter_count"`
	Series        string    `structs:"series"        json:"series"        db:"series"`
	LibraryID     int       `structs:"library_id"    json:"libraryId"     db:"library_id"`
	Path          string    `structs:"path"          json:"path"          db:"path"`
	Hash          string    `structs:"hash"          json:"hash"          db:"hash"`
	Size          int64     `structs:"size"          json:"size"          db:"size"`
	Starred       string    `structs:"-"             json:"starred,omitempty"  db:"-"` // populated at query time, not stored in audiobook table
	CreatedAt     time.Time `structs:"created_at"    json:"createdAt"     db:"created_at"`
	UpdatedAt     time.Time `structs:"updated_at"    json:"updatedAt"     db:"updated_at"`
}

type AudiobookChapter struct {
	ID            string `structs:"id"             json:"id"            db:"id"`
	AudiobookID   string `structs:"audiobook_id"   json:"audiobookId"   db:"audiobook_id"`
	Title         string `structs:"title"          json:"title"         db:"title"`
	ChapterNumber int    `structs:"chapter_number" json:"chapterNumber" db:"chapter_number"`
	Duration      int    `structs:"duration"       json:"duration"      db:"duration"`
	Format        string `structs:"format"         json:"format"        db:"format"`
	FileSize      int64  `structs:"file_size"      json:"fileSize"      db:"file_size"`
	Path          string `structs:"path"           json:"path"          db:"path"`
	CreatedAt     time.Time `structs:"created_at"  json:"createdAt"     db:"created_at"`
}

type AudiobookProgress struct {
	ID            string    `structs:"id"             json:"id"            db:"id"`
	UserID        string    `structs:"user_id"        json:"userId"        db:"user_id"`
	AudiobookID   string    `structs:"audiobook_id"   json:"audiobookId"   db:"audiobook_id"`
	ChapterID     string    `structs:"chapter_id"     json:"chapterId"     db:"chapter_id"`
	ChapterNumber int       `structs:"chapter_number" json:"chapterNumber" db:"chapter_number"`
	Position      int       `structs:"position"       json:"position"      db:"position"`
	PlaybackSpeed float64   `structs:"playback_speed" json:"playbackSpeed" db:"playback_speed"`
	SkipIntro     int       `structs:"skip_intro"     json:"skipIntro"     db:"skip_intro"`
	SkipOutro     int       `structs:"skip_outro"     json:"skipOutro"     db:"skip_outro"`
	Completed     bool      `structs:"completed"      json:"completed"     db:"completed"`
	LastPlayedAt  time.Time `structs:"last_played_at" json:"lastPlayedAt"  db:"last_played_at"`
}

type AudiobookBookmark struct {
	ID          string    `structs:"id"           json:"id"          db:"id"`
	UserID      string    `structs:"user_id"      json:"userId"      db:"user_id"`
	AudiobookID string    `structs:"audiobook_id" json:"audiobookId" db:"audiobook_id"`
	ChapterID   string    `structs:"chapter_id"   json:"chapterId"   db:"chapter_id"`
	Position    int       `structs:"position"     json:"position"    db:"position"`
	Title       string    `structs:"title"        json:"title"       db:"title"`
	CreatedAt   time.Time `structs:"created_at"   json:"createdAt"   db:"created_at"`
}

type AudiobookFavorite struct {
	ID          string    `structs:"id"           json:"id"          db:"id"`
	UserID      string    `structs:"user_id"      json:"userId"      db:"user_id"`
	AudiobookID string    `structs:"audiobook_id" json:"audiobookId" db:"audiobook_id"`
	CreatedAt   time.Time `structs:"created_at"   json:"createdAt"   db:"created_at"`
}

type Audiobooks []Audiobook
type AudiobookChapters []AudiobookChapter

type AudiobookRepository interface {
	Get(id string) (*Audiobook, error)
	GetAll(options ...QueryOptions) (Audiobooks, error)
	Count(options ...QueryOptions) (int64, error)
	Put(book *Audiobook) error
	Delete(id string) error

	GetChapters(audiobookID string) (AudiobookChapters, error)
	GetChapter(id string) (*AudiobookChapter, error)
	PutChapter(chapter *AudiobookChapter) error
	DeleteChapters(audiobookID string) error

	GetProgress(userID, audiobookID string) (*AudiobookProgress, error)
	SaveProgress(progress *AudiobookProgress) error
	GetAllProgress() ([]AudiobookProgress, error)

	GetBookmarks(userID, audiobookID string) ([]AudiobookBookmark, error)
	SaveBookmark(bookmark *AudiobookBookmark) error
	DeleteBookmark(id string) error
	GetAllBookmarks() ([]AudiobookBookmark, error)

	Star(userID, audiobookID string) error
	Unstar(userID, audiobookID string) error
	IsStarred(userID, audiobookID string) (bool, error)
	GetStarredAt(userID, audiobookID string) (string, error)
	GetStarred(userID string) (Audiobooks, error)
}

// [LeChenMusic-END:audiobook]
