package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAssignDefaultLibrariesToExistingUsers, downAssignDefaultLibrariesToExistingUsers)
}

// 修复“网盘音乐搜不到”：新媒体库（如 openlist 网盘库）创建时只自动发给了管理员，
// 存量普通用户没有对应的 user_library 行，服务端按库过滤后整个库对他们不可见
// （列表/搜索/专辑/艺人都查不到）。这里把 default_new_users 的媒体库补发给所有
// 还没有它的用户；管理员后续仍可在用户管理里单独收回授权。
func upAssignDefaultLibrariesToExistingUsers(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_library (user_id, library_id)
SELECT u.id, l.id
FROM user u
CROSS JOIN library l
WHERE l.default_new_users = true
  AND NOT EXISTS (
      SELECT 1 FROM user_library ul
      WHERE ul.user_id = u.id AND ul.library_id = l.id
  )`)
	return err
}

func downAssignDefaultLibrariesToExistingUsers(ctx context.Context, tx *sql.Tx) error {
	// 补发的授权没有安全的逆操作：回退可能误删管理员手工配置的授权，故保留数据。
	return nil
}
