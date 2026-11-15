package online

import (
	"context"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// Permission modes for the online-download feature. They mirror
// conf.Server.OnlineDownload.Permission, but can be overridden at runtime
// through the admin UI (stored in the property table).
const (
	// PermissionOpen lets every logged-in user download.
	PermissionOpen = "open"
	// PermissionAdmin restricts downloads to administrators.
	PermissionAdmin = "admin-or-approved"
	// PermissionQuota lets every logged-in user download, up to DailyQuota
	// songs per day.
	PermissionQuota = "quota"
)

// Property-table keys holding the runtime overrides. The `conf` value is used
// as the default when no override was ever stored.
const (
	SettingKeyPermission  = "OnlineDownload.Permission"
	SettingKeyDailyQuota  = "OnlineDownload.DailyQuota"
	defaultPermissionMode = PermissionAdmin
)

// Settings is the runtime-editable part of the online-download configuration.
// It is read on every request, so changes made by an admin take effect
// immediately and without a restart.
type Settings struct {
	Enabled    bool   `json:"enabled"`
	Permission string `json:"permission"`
	DailyQuota int    `json:"dailyQuota"`
}

// LoadSettings resolves the effective settings: property-table overrides first,
// then the value from the configuration file / environment.
func LoadSettings(ctx context.Context, ds model.DataStore) Settings {
	s := Settings{
		Enabled:    conf.Server.OnlineDownload.Enabled,
		Permission: NormalizePermission(conf.Server.OnlineDownload.Permission),
		DailyQuota: conf.Server.OnlineDownload.DailyQuota,
	}
	if ds == nil {
		return s
	}
	repo := ds.Property(ctx)
	if repo == nil {
		return s
	}
	if raw, err := repo.DefaultGet(SettingKeyPermission, ""); err != nil {
		log.Warn(ctx, "online: could not load permission setting", err)
	} else if raw != "" {
		s.Permission = NormalizePermission(raw)
	}
	if raw, err := repo.DefaultGet(SettingKeyDailyQuota, ""); err != nil {
		log.Warn(ctx, "online: could not load daily quota setting", err)
	} else if raw != "" {
		if n, convErr := strconv.Atoi(strings.TrimSpace(raw)); convErr == nil && n >= 0 {
			s.DailyQuota = n
		}
	}
	return s
}

// SaveSettings persists the runtime overrides. Only the permission mode and the
// daily quota are editable; the master switch stays in the configuration file.
func SaveSettings(ctx context.Context, ds model.DataStore, s Settings) (Settings, error) {
	if ds == nil || ds.Property(ctx) == nil {
		return s, model.ErrNotAvailable
	}
	repo := ds.Property(ctx)
	permission := NormalizePermission(s.Permission)
	quota := s.DailyQuota
	if quota < 0 {
		quota = 0
	}
	if err := repo.Put(SettingKeyPermission, permission); err != nil {
		return s, err
	}
	if err := repo.Put(SettingKeyDailyQuota, strconv.Itoa(quota)); err != nil {
		return s, err
	}
	return LoadSettings(ctx, ds), nil
}

// NormalizePermission maps anything unknown/empty to the safest mode
// (admin-only), so a missing or corrupted value never opens downloads up.
func NormalizePermission(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionOpen:
		return PermissionOpen
	case PermissionQuota:
		return PermissionQuota
	case PermissionAdmin:
		return PermissionAdmin
	case "":
		return defaultPermissionMode
	default:
		return defaultPermissionMode
	}
}

// ValidPermission reports whether value is one of the accepted modes.
func ValidPermission(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionOpen, PermissionQuota, PermissionAdmin:
		return true
	}
	return false
}

// Access describes what the current user may do. It is returned by the
// settings endpoint so both the web UI and the mobile app can render the same
// state without duplicating the permission rules.
type Access struct {
	IsAdmin        bool `json:"isAdmin"`
	CanDownload    bool `json:"canDownload"`
	UsedToday      int  `json:"usedToday"`
	RemainingQuota int  `json:"remainingQuota"` // -1 = unlimited
}

// CheckAccess evaluates the permission mode for one user.
func (s Settings) CheckAccess(isAdmin bool, usedToday int) Access {
	access := Access{IsAdmin: isAdmin, UsedToday: usedToday, RemainingQuota: -1}
	switch s.Permission {
	case PermissionOpen:
		access.CanDownload = true
	case PermissionQuota:
		access.CanDownload = true
		if s.DailyQuota > 0 {
			remaining := s.DailyQuota - usedToday
			if remaining < 0 {
				remaining = 0
			}
			access.RemainingQuota = remaining
			access.CanDownload = remaining > 0
		}
	case PermissionAdmin:
		access.CanDownload = isAdmin
	default:
		access.CanDownload = isAdmin
	}
	return access
}
