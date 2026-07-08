package persistence

import (
	"context"
	"crypto/md5"
	"fmt"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/navidrome/navidrome/utils"
	"github.com/pocketbase/dbx"
)

// [LeChenMusic-START:audiobook]

type audiobookRepository struct {
	sqlRepository
}

func NewAudiobookRepository(ctx context.Context, db dbx.Builder) model.AudiobookRepository {
	r := &audiobookRepository{}
	r.ctx = ctx
	r.db = db
	r.registerModel(&model.Audiobook{}, map[string]filterFunc{
		"title":    containsFilter("title"),
		"author":   containsFilter("author"),
		"narrator": containsFilter("narrator"),
		"genre":    containsFilter("genre"),
		"series":   containsFilter("series"),
	})
	return r
}

// ─── Audiobook CRUD ──────────────────────────────────────

func (r *audiobookRepository) Get(id string) (*model.Audiobook, error) {
	sel := r.newSelect().Where(Eq{"id": id}).Columns("*")
	res := model.Audiobook{}
	err := r.queryOne(sel, &res)
	return &res, err
}

func (r *audiobookRepository) GetAll(options ...model.QueryOptions) (model.Audiobooks, error) {
	sel := r.newSelect(options...).Columns("*")
	res := model.Audiobooks{}
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *audiobookRepository) Count(options ...model.QueryOptions) (int64, error) {
	sql := r.newSelect()
	return r.count(sql, options...)
}

func (r *audiobookRepository) Put(book *model.Audiobook) error {
	book.UpdatedAt = time.Now()
	if book.ID == "" {
		book.CreatedAt = time.Now()
		book.ID = id.NewRandom()
	}
	_, err := r.put(book.ID, book)
	return err
}

func (r *audiobookRepository) Delete(id string) error {
	return r.delete(Eq{"id": id})
}

// ─── Chapter CRUD ────────────────────────────────────────

func (r *audiobookRepository) GetChapters(audiobookID string) (model.AudiobookChapters, error) {
	sel := r.newSelect().From("audiobook_chapter").
		Where(Eq{"audiobook_id": audiobookID}).
		OrderBy("chapter_number").
		Columns("*")
	res := model.AudiobookChapters{}
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *audiobookRepository) GetChapter(id string) (*model.AudiobookChapter, error) {
	sel := r.newSelect().From("audiobook_chapter").
		Where(Eq{"id": id}).
		Columns("*")
	res := model.AudiobookChapter{}
	err := r.queryOne(sel, &res)
	return &res, err
}

func (r *audiobookRepository) PutChapter(chapter *model.AudiobookChapter) error {
	if chapter.ID == "" {
		chapter.ID = id.NewRandom()
	}
	chapter.CreatedAt = time.Now()
	values, err := toSQLArgs(chapter)
	if err != nil {
		return err
	}
	sq := Insert("audiobook_chapter").SetMap(values)
	_, err = r.executeSQL(sq)
	if err == nil {
		r.updateAudiobookStats(chapter.AudiobookID)
	}
	return err
}

func (r *audiobookRepository) DeleteChapters(audiobookID string) error {
	sq := Delete("audiobook_chapter").Where(Eq{"audiobook_id": audiobookID})
	_, err := r.executeSQL(sq)
	return err
}

func (r *audiobookRepository) updateAudiobookStats(audiobookID string) {
	var result struct {
		Count    int   `db:"cnt"`
		Duration int   `db:"dur"`
		Size     int64 `db:"sz"`
	}
	_ = r.db.NewQuery(`
		SELECT count(*) as cnt, coalesce(sum(duration),0) as dur, coalesce(sum(file_size),0) as sz
		FROM audiobook_chapter WHERE audiobook_id = {:id}
	`).Bind(dbx.Params{"id": audiobookID}).One(&result)

	sq := Update("audiobook").
		Set("chapter_count", result.Count).
		Set("total_duration", result.Duration).
		Set("size", result.Size).
		Set("updated_at", time.Now()).
		Where(Eq{"id": audiobookID})
	_, _ = r.executeSQL(sq)
}

// ─── Progress ────────────────────────────────────────────

func (r *audiobookRepository) GetProgress(userID, audiobookID string) (*model.AudiobookProgress, error) {
	sel := r.newSelect().From("audiobook_progress").
		Where(And{
			Eq{"user_id": userID},
			Eq{"audiobook_id": audiobookID},
		}).Columns("*")
	res := model.AudiobookProgress{}
	err := r.queryOne(sel, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *audiobookRepository) SaveProgress(progress *model.AudiobookProgress) error {
	if progress.ID == "" {
		progress.ID = id.NewRandom()
	}
	progress.LastPlayedAt = time.Now()
	// Use explicit table name to avoid writing to 'audiobook' table
	values, err := toSQLArgs(progress)
	if err != nil {
		return err
	}
	update := Update("audiobook_progress").Where(Eq{"id": progress.ID}).SetMap(filterUpdateValues(values, progress.ID))
	count, err := r.executeSQL(update)
	if err != nil {
		return err
	}
	if count == 0 {
		insert := Insert("audiobook_progress").SetMap(values)
		_, err = r.executeSQL(insert)
	}
	return err
}

func (r *audiobookRepository) GetAllProgress() ([]model.AudiobookProgress, error) {
	sel := r.newSelect().From("audiobook_progress").Columns("*")
	var res []model.AudiobookProgress
	err := r.queryAll(sel, &res)
	return res, err
}

// ─── Bookmarks ───────────────────────────────────────────

func (r *audiobookRepository) GetBookmarks(userID, audiobookID string) ([]model.AudiobookBookmark, error) {
	sel := r.newSelect().From("audiobook_bookmark").
		Where(And{
			Eq{"user_id": userID},
			Eq{"audiobook_id": audiobookID},
		}).
		OrderBy("created_at DESC").
		Columns("*")
	res := []model.AudiobookBookmark{}
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *audiobookRepository) SaveBookmark(bookmark *model.AudiobookBookmark) error {
	if bookmark.ID == "" {
		bookmark.ID = id.NewRandom()
	}
	bookmark.CreatedAt = time.Now()
	values, err := toSQLArgs(bookmark)
	if err != nil {
		return err
	}
	update := Update("audiobook_bookmark").Where(Eq{"id": bookmark.ID}).SetMap(filterUpdateValues(values, bookmark.ID))
	count, err := r.executeSQL(update)
	if err != nil {
		return err
	}
	if count == 0 {
		insert := Insert("audiobook_bookmark").SetMap(values)
		_, err = r.executeSQL(insert)
	}
	return err
}

func (r *audiobookRepository) GetAllBookmarks() ([]model.AudiobookBookmark, error) {
	sel := r.newSelect().From("audiobook_bookmark").Columns("*")
	var res []model.AudiobookBookmark
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *audiobookRepository) DeleteBookmark(id string) error {
	return r.delete(Eq{"id": id})
}

// ─── Favorites ───────────────────────────────────────────

func (r *audiobookRepository) Star(userID, audiobookID string) error {
	fav := &model.AudiobookFavorite{
		ID:          id.NewRandom(),
		UserID:      userID,
		AudiobookID: audiobookID,
		CreatedAt:   time.Now(),
	}
	_, err := r.put(fav.ID, fav)
	return err
}

func (r *audiobookRepository) Unstar(userID, audiobookID string) error {
	sq := Delete("audiobook_favorite").Where(And{
		Eq{"user_id": userID},
		Eq{"audiobook_id": audiobookID},
	})
	_, err := r.executeSQL(sq)
	return err
}

func (r *audiobookRepository) IsStarred(userID, audiobookID string) (bool, error) {
	sel := r.newSelect().From("audiobook_favorite").
		Where(And{
			Eq{"user_id": userID},
			Eq{"audiobook_id": audiobookID},
		}).Columns("count(*)")
	var count int64
	err := r.queryOne(sel, &count)
	return count > 0, err
}

func (r *audiobookRepository) GetStarred(userID string) (model.Audiobooks, error) {
	sel := r.newSelect().From("audiobook").
		Join("audiobook_favorite ON audiobook.id = audiobook_favorite.audiobook_id").
		Where(Eq{"audiobook_favorite.user_id": userID}).
		OrderBy("audiobook_favorite.created_at DESC").
		Columns("audiobook.*")
	res := model.Audiobooks{}
	err := r.queryAll(sel, &res)
	return res, err
}

// ─── Helpers ─────────────────────────────────────────────

func AudiobookHash(path string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(path)))
}

func AudiobookCoverExists(path string) bool {
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.jpeg", "folder.png"} {
		if utils.FileExists(path + "/" + name) {
			return true
		}
	}
	return false
}

// [LeChenMusic-END:audiobook]

