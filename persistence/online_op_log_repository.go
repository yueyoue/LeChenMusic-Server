package persistence

import (
	"context"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/pocketbase/dbx"
)

type onlineOpLogRepository struct {
	sqlRepository
}

func NewOnlineOpLogRepository(ctx context.Context, db dbx.Builder) model.OnlineOpLogRepository {
	r := &onlineOpLogRepository{}
	r.ctx = ctx
	r.db = db
	r.registerModel(&model.OnlineOpLog{}, nil)
	return r
}

func (r *onlineOpLogRepository) Add(entry *model.OnlineOpLog) error {
	if entry.ID == "" {
		entry.ID = id.NewRandom()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	values, err := toSQLArgs(entry)
	if err != nil {
		return err
	}
	sq := Insert("online_op_log").SetMap(values)
	_, err = r.executeSQL(sq)
	return err
}

func (r *onlineOpLogRepository) GetAll() (model.OnlineOpLogs, error) {
	sq := r.newSelect().
		From("online_op_log").
		Columns("*").
		OrderBy("created_at DESC").
		Limit(500)
	var res model.OnlineOpLogs
	err := r.queryAll(sq, &res)
	return res, err
}

func (r *onlineOpLogRepository) DeleteAll() error {
	sq := Delete("online_op_log")
	_, err := r.executeSQL(sq)
	return err
}
