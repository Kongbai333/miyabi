package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ppxb/miyabi/internal/tasks"
)

// Ent cannot describe expression indexes. Keep the existing JSON storage and
// index the stable scope/hash fields used by both offline history queries.
// Other task types do not pay for these indexes or carry their larger documents.
func createTaskHistoryIndexes(ctx context.Context, database *sql.DB) error {
	for _, statement := range []string{
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS task_offline_source_history ON tasks (
			json_extract(payload, '$.%s'), json_extract(payload, '$.%s'),
			json_type(payload, '$.%s'), json_extract(payload, '$.%s'), id DESC
		) WHERE type = '%s'`, tasks.PathAccountID, tasks.PathDirectoryID, tasks.PathHash, tasks.PathHash, tasks.KindOffline),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS task_offline_movie_history ON tasks (
			json_extract(payload, '$.%s'), json_extract(payload, '$.%s'),
			json_type(payload, '$.%s'), json_extract(payload, '$.%s'), id DESC
		) WHERE type = '%s'`, tasks.PathAccountID, tasks.PathJavDBID, tasks.PathHash, tasks.PathHash, tasks.KindOffline),
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create offline task history index: %w", err)
		}
	}
	return nil
}
