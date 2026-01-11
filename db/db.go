package db

import (
	"database/sql"

	_ "modernc.org/sqlite"

	"github.com/Tk21111/whiteboard_server/config"
)

type eventWriter struct {
	db *sql.DB
	ch chan config.Event
}

var (
	W *eventWriter
)

func NewWriter(dbPath string) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}

	// Optional but HIGHLY recommended for SQLite
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		panic(err)
	}

	// Create table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER NOT NULL,
			room_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			op TEXT NOT NULL,
			payload BLOB NOT NULL,
			created_at INTEGER NOT NULL
		);
	`)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_events_room_clock
		ON events(room_id, id);
	`)
	if err != nil {
		panic(err)
	}

	W = &eventWriter{
		db: db,
		ch: make(chan config.Event, 8192),
	}

	go W.loop()
}

func (w *eventWriter) loop() {
	stmt, err := w.db.Prepare(`
		INSERT INTO events
		(id, room_id, user_id, entity_id, op, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`)
	if err != nil {
		panic(err)
	}
	defer stmt.Close()

	for e := range w.ch {
		_, err := stmt.Exec(
			e.ID,
			e.RoomID,
			e.UserID,
			e.EntityID,
			e.Op,
			e.Payload,
			e.CreatedAt,
		)
		if err != nil {
			// TODO: logging / retry / DLQ
			continue
		}
	}
}

func WriteEvent(e config.Event) {
	if W == nil {
		return
	}

	select {
	case W.ch <- e:
	default:
		// channel full → drop or log
	}
}

func GetEvent(roomID string, id string) ([]config.Event, error) {
	rows, err := W.db.Query(`
		SELECT id, room_id, user_id, entity_id, op, payload, created_at
		FROM events
		WHERE room_id = ? AND id > ?
		ORDER BY id ASC
	`, roomID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []config.Event{}

	for rows.Next() {
		var e config.Event
		if err := rows.Scan(
			&e.ID,
			&e.RoomID,
			&e.UserID,
			&e.EntityID,
			&e.Op,
			&e.Payload,
			&e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	return events, nil
}
