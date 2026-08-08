package persistence

import (
	"context"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/pocketbase/dbx"
)

type userDeviceRepository struct {
	sqlRepository
}

func NewUserDeviceRepository(ctx context.Context, db dbx.Builder) model.UserDeviceRepository {
	r := &userDeviceRepository{}
	r.ctx = ctx
	r.db = db
	r.registerModel(&model.UserDevice{}, nil)
	return r
}

func (r *userDeviceRepository) Upsert(device *model.UserDevice) error {
	now := time.Now()
	if device.LastSeenAt.IsZero() {
		device.LastSeenAt = now
	}

	// Try to find existing record by (user_id, device_id)
	var existing model.UserDevice
	sq := StatementBuilder.PlaceholderFormat(Question).
		Select("*").From("user_device").
		Where(And{
			Eq{"user_id": device.UserID},
			Eq{"device_id": device.DeviceID},
		})
	err := r.queryOne(sq, &existing)
	if err == nil && existing.ID != "" {
		// Update existing
		update := Update("user_device").
			Set("device_info", device.DeviceInfo).
			Set("app_version", device.AppVersion).
			Set("last_seen_at", now).
			Where(Eq{"id": existing.ID})
		_, err = r.executeSQL(update)
		return err
	}

	// Insert new
	if device.ID == "" {
		device.ID = id.NewRandom()
	}
	device.CreatedAt = now
	values, err := toSQLArgs(device)
	if err != nil {
		return err
	}
	insert := Insert("user_device").SetMap(values)
	_, err = r.executeSQL(insert)
	return err
}

func (r *userDeviceRepository) GetByUser(userID string) (model.UserDevices, error) {
	sq := r.newSelect().
		From("user_device").
		Where(Eq{"user_id": userID}).
		OrderBy("last_seen_at DESC").
		Columns("*")
	var res model.UserDevices
	err := r.queryAll(sq, &res)
	return res, err
}

func (r *userDeviceRepository) VersionStats() ([]model.VersionStat, error) {
	var results []model.VersionStat
	err := r.db.NewQuery(`
		SELECT
			app_version,
			count(DISTINCT user_id) AS user_count
		FROM user_device
		GROUP BY app_version
		ORDER BY user_count DESC
	`).All(&results)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []model.VersionStat{}
	}
	return results, nil
}
