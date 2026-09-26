package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const (
	maxSyncBatch  = 500 // tasks per push request / pull page
	pullLimitMax  = 500
	pullLimitDef  = 500
)

// handleSyncPush upserts a batch of tasks for the authenticated user.
// Last-write-wins: an incoming row only overwrites the stored one when its
// updatedAt is strictly greater. `applied` counts rows that were actually
// inserted or updated.
func (s *server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
	var req SyncPushRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Tasks) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "tasks must not be empty"})
		return
	}
	if len(req.Tasks) > maxSyncBatch {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "too many tasks in one push"})
		return
	}
	for _, t := range req.Tasks {
		if t.ID == "" || t.UpdatedAt <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "each task requires id and updatedAt"})
			return
		}
	}

	user := userFromContext(r)
	batch := &pgx.Batch{}
	for _, t := range req.Tasks {
		batch.Queue(
			`INSERT INTO tasks (user_id, id, created, title, description, type,
				dead_line, dead_line_ms, status, work, updated_at, deleted)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			 ON CONFLICT (user_id, id) DO UPDATE SET
				created = EXCLUDED.created,
				title = EXCLUDED.title,
				description = EXCLUDED.description,
				type = EXCLUDED.type,
				dead_line = EXCLUDED.dead_line,
				dead_line_ms = EXCLUDED.dead_line_ms,
				status = EXCLUDED.status,
				work = EXCLUDED.work,
				updated_at = EXCLUDED.updated_at,
				deleted = EXCLUDED.deleted
			 WHERE tasks.updated_at < EXCLUDED.updated_at`,
			user.Sub, t.ID, t.Created, t.Title, t.Description, t.Type,
			t.DeadLine, t.DeadLineMs, t.Status, t.Work, t.UpdatedAt, t.Deleted,
		)
	}

	results := s.db.SendBatch(r.Context(), batch)
	applied := 0
	for i := 0; i < len(req.Tasks); i++ {
		tag, err := results.Exec()
		if err != nil {
			log.Printf("sync push row %d: %v", i, err)
			results.Close()
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "db error"})
			return
		}
		// RowsAffected is 0 when the ON CONFLICT guard rejected a stale row.
		applied += int(tag.RowsAffected())
	}
	if err := results.Close(); err != nil {
		log.Printf("sync push: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}

	writeJSON(w, http.StatusOK, SyncPushResponse{Applied: applied})
}

// handleSyncPull returns changed rows (tombstones included) ordered by
// updatedAt ascending. `cursor` is the updatedAt of the last returned row;
// pass it back as `since` on the next call.
func (s *server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	since := int64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil || parsed < 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid since"})
			return
		}
		since = parsed
	}
	limit := pullLimitDef
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 || parsed > pullLimitMax {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid limit"})
			return
		}
		limit = parsed
	}

	user := userFromContext(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT id, created, title, description, type, dead_line, dead_line_ms,
			status, work, updated_at, deleted
		 FROM tasks
		 WHERE user_id = $1 AND updated_at > $2
		 ORDER BY updated_at ASC, id ASC
		 LIMIT $3`,
		user.Sub, since, limit,
	)
	if err != nil {
		log.Printf("sync pull: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	defer rows.Close()

	tasks := make([]SyncTask, 0, limit)
	for rows.Next() {
		var t SyncTask
		if err := rows.Scan(&t.ID, &t.Created, &t.Title, &t.Description, &t.Type,
			&t.DeadLine, &t.DeadLineMs, &t.Status, &t.Work, &t.UpdatedAt, &t.Deleted); err != nil {
			log.Printf("sync pull scan: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "db error"})
			return
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		log.Printf("sync pull rows: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}

	cursor := since
	if len(tasks) > 0 {
		cursor = tasks[len(tasks)-1].UpdatedAt
	}
	writeJSON(w, http.StatusOK, SyncPullResponse{Tasks: tasks, Cursor: cursor})
}
