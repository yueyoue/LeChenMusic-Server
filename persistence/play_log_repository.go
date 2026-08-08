package persistence

import (
	"context"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/pocketbase/dbx"
)

type playLogRepository struct {
	sqlRepository
}

func NewPlayLogRepository(ctx context.Context, db dbx.Builder) model.PlayLogRepository {
	r := &playLogRepository{}
	r.ctx = ctx
	r.db = db
	r.registerModel(&model.PlayLog{}, nil)
	return r
}

func (r *playLogRepository) Add(log *model.PlayLog) error {
	if log.ID == "" {
		log.ID = id.NewRandom()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	if log.PlayDate.IsZero() {
		log.PlayDate = time.Now()
	}
	values, err := toSQLArgs(log)
	if err != nil {
		return err
	}
	sq := Insert("play_log").SetMap(values)
	_, err = r.executeSQL(sq)
	return err
}

func (r *playLogRepository) GetByUser(userID string, max int) (model.PlayLogs, error) {
	sq := r.newSelect().
		From("play_log").
		Where(Eq{"user_id": userID}).
		OrderBy("play_date DESC")
	if max > 0 {
		sq = sq.Limit(uint64(max))
	}
	sq = sq.Columns("*")
	var res model.PlayLogs
	err := r.queryAll(sq, &res)
	return res, err
}

func (r *playLogRepository) GetUserStats(userID string) (*model.UserPlayStats, error) {
	var result model.UserPlayStats
	err := r.db.NewQuery(`
		SELECT
			coalesce(sum(duration), 0) AS total_duration,
			count(*) AS total_count,
			coalesce(sum(CASE WHEN item_type = 'music' THEN duration ELSE 0 END), 0) AS music_duration,
			count(CASE WHEN item_type = 'music' THEN 1 END) AS music_count,
			coalesce(sum(CASE WHEN item_type = 'audiobook' THEN duration ELSE 0 END), 0) AS audiobook_duration,
			count(CASE WHEN item_type = 'audiobook' THEN 1 END) AS audiobook_count
		FROM play_log
		WHERE user_id = {:uid}
	`).Bind(dbx.Params{"uid": userID}).One(&result)
	if err != nil {
		return nil, err
	}
	result.UserID = userID
	// Look up user name
	var userName string
	err = r.db.NewQuery(`SELECT user_name FROM "user" WHERE id = {:uid}`).
		Bind(dbx.Params{"uid": userID}).Row(&userName)
	if err == nil {
		result.UserName = userName
	}
	return &result, nil
}

func (r *playLogRepository) GetAllUsersStats() ([]model.UserPlayStats, error) {
	var results []model.UserPlayStats
	err := r.db.NewQuery(`
		SELECT
			pl.user_id,
			coalesce(u.user_name, '') AS user_name,
			coalesce(sum(pl.duration), 0) AS total_duration,
			count(*) AS total_count,
			coalesce(sum(CASE WHEN pl.item_type = 'music' THEN pl.duration ELSE 0 END), 0) AS music_duration,
			count(CASE WHEN pl.item_type = 'music' THEN 1 END) AS music_count,
			coalesce(sum(CASE WHEN pl.item_type = 'audiobook' THEN pl.duration ELSE 0 END), 0) AS audiobook_duration,
			count(CASE WHEN pl.item_type = 'audiobook' THEN 1 END) AS audiobook_count
		FROM play_log pl
		LEFT JOIN "user" u ON u.id = pl.user_id
		GROUP BY pl.user_id
		ORDER BY total_duration DESC
	`).All(&results)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []model.UserPlayStats{}
	}
	return results, nil
}
