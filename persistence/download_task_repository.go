package persistence

import (
	"context"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/pocketbase/dbx"
)

type downloadTaskRepository struct {
	sqlRepository
}

func NewDownloadTaskRepository(ctx context.Context, db dbx.Builder) model.DownloadTaskRepository {
	r := &downloadTaskRepository{}
	r.ctx = ctx
	r.db = db
	r.registerModel(&model.DownloadTask{}, nil)
	return r
}

func (r *downloadTaskRepository) Add(task *model.DownloadTask) error {
	if task.ID == "" {
		task.ID = id.NewRandom()
	}
	if task.Status == "" {
		task.Status = model.DownloadStatusQueued
	}
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	values, err := toSQLArgs(task)
	if err != nil {
		return err
	}
	sq := Insert("download_task").SetMap(values)
	_, err = r.executeSQL(sq)
	return err
}

func (r *downloadTaskRepository) Update(task *model.DownloadTask) error {
	task.UpdatedAt = time.Now()
	values, err := toSQLArgs(task)
	if err != nil {
		return err
	}
	sq := Update("download_task").SetMap(values).Where(Eq{"id": task.ID})
	_, err = r.executeSQL(sq)
	return err
}

func (r *downloadTaskRepository) Get(id string) (*model.DownloadTask, error) {
	sq := r.newSelect().
		From("download_task").
		Columns("*").
		Where(Eq{"id": id})
	var res model.DownloadTask
	err := r.queryOne(sq, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *downloadTaskRepository) GetAll() (model.DownloadTasks, error) {
	sq := r.newSelect().
		From("download_task").
		Columns("*").
		OrderBy("created_at DESC").
		Limit(500)
	var res model.DownloadTasks
	err := r.queryAll(sq, &res)
	return res, err
}

func (r *downloadTaskRepository) Delete(taskID string) error {
	sq := Delete("download_task").Where(Eq{"id": taskID})
	_, err := r.executeSQL(sq)
	return err
}
