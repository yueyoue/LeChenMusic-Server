package model

import (
	"time"
)

// PlayLog records a single play/pause event reported by the APP.
type PlayLog struct {
	ID         string    `structs:"id"          json:"id"          db:"id"`
	UserID     string    `structs:"user_id"     json:"userId"      db:"user_id"`
	ItemType   string    `structs:"item_type"   json:"itemType"    db:"item_type"`   // "music" or "audiobook"
	ItemID     string    `structs:"item_id"     json:"itemId"      db:"item_id"`
	ItemTitle  string    `structs:"item_title"  json:"itemTitle"   db:"item_title"`
	ItemArtist string    `structs:"item_artist" json:"itemArtist"  db:"item_artist"`
	Duration   int       `structs:"duration"    json:"duration"    db:"duration"`     // seconds
	PlayDate   time.Time `structs:"play_date"   json:"playDate"    db:"play_date"`
	CreatedAt  time.Time `structs:"created_at"  json:"createdAt"   db:"created_at"`
}

// UserPlayStats holds aggregated play statistics for a single user.
type UserPlayStats struct {
	UserID          string `structs:"user_id"           json:"userId"          db:"user_id"`
	UserName        string `structs:"user_name"         json:"userName"        db:"user_name"`
	TotalDuration   int    `structs:"total_duration"    json:"totalDuration"   db:"total_duration"`   // seconds
	TotalCount      int    `structs:"total_count"       json:"totalCount"      db:"total_count"`
	MusicDuration   int    `structs:"music_duration"    json:"musicDuration"   db:"music_duration"`
	MusicCount      int    `structs:"music_count"       json:"musicCount"      db:"music_count"`
	AudiobookDuration int  `structs:"audiobook_duration" json:"audiobookDuration" db:"audiobook_duration"`
	AudiobookCount  int    `structs:"audiobook_count"   json:"audiobookCount"  db:"audiobook_count"`
}

type PlayLogs []PlayLog

// PlayLogRepository provides methods for storing and querying play logs.
type PlayLogRepository interface {
	// Add inserts a new play log entry.
	Add(log *PlayLog) error
	// GetByUser returns play logs for a specific user, ordered by play_date desc.
	GetByUser(userID string, max int) (PlayLogs, error)
	// GetUserStats returns aggregated play stats for a single user.
	GetUserStats(userID string) (*UserPlayStats, error)
	// GetAllUsersStats returns aggregated play stats for every user who has play logs.
	GetAllUsersStats() ([]UserPlayStats, error)
}
