package model

import "time"

// Online op-log action values.
const (
	OpLogDownloadQueued = "download_queued"
	OpLogDownloadDone   = "download_done"
	OpLogDownloadFailed = "download_failed"
	OpLogDelete         = "delete"
	OpLogRescrape       = "rescrape"
)

// OnlineOpLog records one operation in the online download & scraping
// subsystem: who did what to which song, when, and how it went.
// Used by the admin "下载管理" page for auditing/verification.
type OnlineOpLog struct {
	ID        string    `structs:"id"         json:"id"         db:"id"`
	UserID    string    `structs:"user_id"    json:"userId"     db:"user_id"`
	UserName  string    `structs:"user_name"  json:"userName"   db:"user_name"`
	Action    string    `structs:"action"     json:"action"     db:"action"`
	SongName  string    `structs:"song_name"  json:"songName"   db:"song_name"`
	Artist    string    `structs:"artist"     json:"artist"     db:"artist"`
	Detail    string    `structs:"detail"     json:"detail"     db:"detail"`
	Success   bool      `structs:"success"    json:"success"    db:"success"`
	CreatedAt time.Time `structs:"created_at" json:"createdAt"  db:"created_at"`
}

type OnlineOpLogs []OnlineOpLog

// OnlineOpLogRepository provides methods for storing and querying op logs.
type OnlineOpLogRepository interface {
	// Add inserts a new log entry (sets ID/created_at if empty).
	Add(entry *OnlineOpLog) error
	// GetAll returns log entries, newest first (bounded).
	GetAll() (OnlineOpLogs, error)
	// DeleteAll clears the log.
	DeleteAll() error
}
