package model

import "time"

// Download task status values.
const (
	DownloadStatusQueued      = "queued"
	DownloadStatusResolving   = "resolving"
	DownloadStatusDownloading = "downloading"
	DownloadStatusTagging     = "tagging" // post-processing: tagging/cover/lyrics/scan
	DownloadStatusDone        = "done"
	DownloadStatusFailed      = "failed"
)

// DownloadTask tracks one "download a song to the server" job.
type DownloadTask struct {
	ID               string    `structs:"id"                json:"id"                db:"id"`
	UserID           string    `structs:"user_id"           json:"userId"            db:"user_id"`
	Source           string    `structs:"source"            json:"source"            db:"source"`
	Platform         string    `structs:"platform"          json:"platform"          db:"platform"`
	SongID           string    `structs:"song_id"           json:"songId"            db:"song_id"`
	SongName         string    `structs:"song_name"         json:"songName"          db:"song_name"`
	Artist           string    `structs:"artist"            json:"artist"            db:"artist"`
	Duration         int       `structs:"duration"          json:"duration"          db:"duration"` // expected song length (seconds) from the source; guards against ad/placeholder audio
	AlbumName        string    `structs:"album_name"        json:"albumName"         db:"album_name"`
	AlbumID          string    `structs:"album_id"          json:"albumId"           db:"album_id"` // upstream album id: resolves album artist + release year
	CoverURL         string    `structs:"cover_url"         json:"coverUrl"          db:"cover_url"`
	RequestedQuality string    `structs:"quality"           json:"quality"           db:"quality"` // e.g. "flac/2000"; empty = default chain
	ActualFormat     string    `structs:"actual_format"     json:"actualFormat"      db:"actual_format"`
	ActualBitrate    int       `structs:"actual_bitrate"    json:"actualBitrate"     db:"actual_bitrate"`
	Status           string    `structs:"status"            json:"status"            db:"status"`
	ProgressBytes    int64     `structs:"progress_bytes"    json:"progressBytes"     db:"progress_bytes"`
	TotalBytes       int64     `structs:"total_bytes"       json:"totalBytes"        db:"total_bytes"`
	FilePath         string    `structs:"file_path"         json:"filePath"          db:"file_path"`
	Error            string    `structs:"error"             json:"error"             db:"error"`
	UpstreamTime     int64     `structs:"upstream_time"     json:"upstreamTime"      db:"upstream_time"` // upstream signature material
	Sign             string    `structs:"sign"              json:"sign"              db:"sign"`
	CreatedAt        time.Time `structs:"created_at"        json:"createdAt"         db:"created_at"`
	UpdatedAt        time.Time `structs:"updated_at"        json:"updatedAt"         db:"updated_at"`
	FinishedAt       time.Time `structs:"finished_at"       json:"finishedAt"        db:"finished_at"`
}

type DownloadTasks []DownloadTask

// DownloadTaskRepository provides methods for storing and querying download tasks.
type DownloadTaskRepository interface {
	// Add inserts a new task (sets ID/status/timestamps if empty).
	Add(task *DownloadTask) error
	// Update persists all fields of an existing task.
	Update(task *DownloadTask) error
	// Get returns a task by ID (model.ErrNotFound if missing).
	Get(id string) (*DownloadTask, error)
	// GetAll returns tasks, newest first.
	GetAll() (DownloadTasks, error)
	// Delete removes a task by ID.
	Delete(id string) error
}
