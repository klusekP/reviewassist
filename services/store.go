package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure Go driver, works with CGO_ENABLED=0
)

// Store is the SQLite database with settings changed in the UI and review results.
type Store struct {
	db   *sql.DB
	path string
}

// ReviewRecord is one stored review of a merge request.
type ReviewRecord struct {
	ID        int64           `json:"id"`
	ProjectID int             `json:"project_id"`
	IID       int             `json:"iid"`
	SHA       string          `json:"sha"`
	Title     string          `json:"title"`
	Verdict   string          `json:"verdict"`
	Score     int             `json:"score"`
	Engine    string          `json:"engine"`
	CreatedAt string          `json:"created_at"`
	Review    json.RawMessage `json:"review"`
}

const schema = `
CREATE TABLE IF NOT EXISTS settings (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS reviews (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	iid        INTEGER NOT NULL,
	sha        TEXT NOT NULL DEFAULT '',
	title      TEXT NOT NULL DEFAULT '',
	verdict    TEXT NOT NULL DEFAULT '',
	score      INTEGER NOT NULL DEFAULT 0,
	engine     TEXT NOT NULL DEFAULT '',
	review     TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reviews_mr ON reviews (project_id, iid, id);
`

// DBPath returns the database location (DB_PATH, default data/codereview.db).
// It is a bootstrap setting, so it can only be set in the environment / .env.
func DBPath() string {
	return envString("DB_PATH", filepath.Join("data", "codereview.db"))
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("cannot create database directory: %v", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; avoids "database is locked"
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot initialise database schema: %v", err)
	}
	_ = os.Chmod(path, 0o600) // contains tokens entered in the UI
	return &Store{db: db, path: path}, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Close() error { return s.db.Close() }

// ---------- settings ----------

func (s *Store) Settings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ApplySettings writes and deletes settings in one transaction.
func (s *Store) ApplySettings(set map[string]string, del []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for k, v := range set {
		if _, err := tx.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, k, v, now); err != nil {
			return err
		}
	}
	for _, k := range del {
		if _, err := tx.Exec(`DELETE FROM settings WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- reviews ----------

func (s *Store) SaveReview(rec *ReviewRecord) error {
	if rec.CreatedAt == "" {
		rec.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.Exec(`INSERT INTO reviews (project_id, iid, sha, title, verdict, score, engine, review, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ProjectID, rec.IID, rec.SHA, rec.Title, rec.Verdict, rec.Score, rec.Engine, string(rec.Review), rec.CreatedAt)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

const reviewColumns = `id, project_id, iid, sha, title, verdict, score, engine, review, created_at`

func scanReviews(rows *sql.Rows) ([]ReviewRecord, error) {
	defer rows.Close()
	out := []ReviewRecord{}
	for rows.Next() {
		var r ReviewRecord
		var review string
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.IID, &r.SHA, &r.Title, &r.Verdict, &r.Score, &r.Engine, &review, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Review = json.RawMessage(review)
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestReviews returns the newest review of each MR (optionally of one project), newest first.
func (s *Store) LatestReviews(projectID, limit int) ([]ReviewRecord, error) {
	q := `SELECT ` + reviewColumns + ` FROM reviews WHERE id IN (
		SELECT MAX(id) FROM reviews WHERE (? = 0 OR project_id = ?) GROUP BY project_id, iid
	) ORDER BY id DESC LIMIT ?`
	rows, err := s.db.Query(q, projectID, projectID, limit)
	if err != nil {
		return nil, err
	}
	return scanReviews(rows)
}

// ReviewHistory returns all reviews of one MR, newest first.
func (s *Store) ReviewHistory(projectID, iid int) ([]ReviewRecord, error) {
	rows, err := s.db.Query(`SELECT `+reviewColumns+` FROM reviews WHERE project_id = ? AND iid = ? ORDER BY id DESC`, projectID, iid)
	if err != nil {
		return nil, err
	}
	return scanReviews(rows)
}

func (s *Store) DeleteReviews(projectID, iid int) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM reviews WHERE project_id = ? AND iid = ?`, projectID, iid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) HasReview(projectID, iid int) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE project_id = ? AND iid = ?`, projectID, iid).Scan(&n)
	return n > 0, err
}
