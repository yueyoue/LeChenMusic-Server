package model

import (
	"time"
)

// UserDevice stores device / app-version information reported by the APP.
type UserDevice struct {
	ID         string    `structs:"id"           json:"id"           db:"id"`
	UserID     string    `structs:"user_id"      json:"userId"       db:"user_id"`
	DeviceID   string    `structs:"device_id"    json:"deviceId"     db:"device_id"`
	DeviceInfo string    `structs:"device_info"  json:"deviceInfo"   db:"device_info"`
	AppVersion string    `structs:"app_version"  json:"appVersion"   db:"app_version"`
	LastSeenAt time.Time `structs:"last_seen_at" json:"lastSeenAt"   db:"last_seen_at"`
	CreatedAt  time.Time `structs:"created_at"   json:"createdAt"    db:"created_at"`
}

// VersionStat holds the user count for a specific app version.
type VersionStat struct {
	AppVersion string `structs:"app_version" json:"appVersion" db:"app_version"`
	UserCount  int    `structs:"user_count"  json:"userCount"  db:"user_count"`
}

type UserDevices []UserDevice

// UserDeviceRepository provides methods for storing and querying user device info.
type UserDeviceRepository interface {
	// Upsert inserts or updates device info. Matches on (user_id, device_id).
	Upsert(device *UserDevice) error
	// GetByUser returns all devices for a specific user.
	GetByUser(userID string) (UserDevices, error)
	// VersionStats returns the number of distinct users per app version.
	VersionStats() ([]VersionStat, error)
}
