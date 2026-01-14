package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Tk21111/whiteboard_server/config"
)

// Operation Types
const (
	OpWriteEvent = iota
	OpDomUpsert
	OpDomRemove
)

// DbJob is a unified struct that can hold data for any DB operation
type DbJob struct {
	Type         int
	Event        config.Event
	Dom          config.DomEvent
	RemoveID     string
	RemoveRoomID string
}

type Writer struct {
	db   *sql.DB
	opCh chan DbJob // Single channel for all writes
}

var (
	W *Writer
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
        PRAGMA busy_timeout = 5000; -- Wait 5s if db is locked
    `); err != nil {
		panic(err)
	}

	// Create events table
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

	// Create dom_objects table
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS dom_objects (
            id TEXT PRIMARY KEY,
            room_id TEXT NOT NULL,
            user_id TEXT NOT NULL,
            kind TEXT NOT NULL, 
            x   REAL NOT NULL,
            y   REAL NOT NULL,
            rot REAL NOT NULL,
            w   REAL NOT NULL,
            h   REAL NOT NULL,
    
            is_removed INTEGER NOT NULL DEFAULT 0,
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL
        );
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_dom_objects_room
        ON dom_objects(room_id);
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_dom_objects_room_active
        ON dom_objects(room_id, is_removed);
    `)
	if err != nil {
		panic(err)
	}

	W = &Writer{
		db: db,
		// Combined buffer size
		opCh: make(chan DbJob, 10000),
	}

	// Start the single unified writer loop
	go W.writerLoop()
}

func (w *Writer) writerLoop() {
	// 1. Prepare Event Statement
	stmtEvent, err := w.db.Prepare(`
        INSERT INTO events
        (id, room_id, user_id, entity_id, op, payload, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `)
	if err != nil {
		panic(err)
	}
	defer stmtEvent.Close()

	// 2. Prepare Dom Upsert Statement
	stmtDom, err := w.db.Prepare(`
        INSERT INTO dom_objects
        (
            id, room_id, user_id, kind,
            x, y, rot, w, h,
            created_at, updated_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ? ,?)
        ON CONFLICT(id) DO UPDATE SET
            x = excluded.x,
            y = excluded.y,
            rot = excluded.rot,
            w = excluded.w,
            h = excluded.h,
            updated_at = excluded.updated_at
    `)
	if err != nil {
		panic(err)
	}
	defer stmtDom.Close()

	// 3. Prepare Dom Remove Statement
	stmtRemove, err := w.db.Prepare(`
        UPDATE dom_objects
        SET
            is_removed = 1,
            updated_at = ?
        WHERE
            id = ?
            AND room_id = ?
            AND is_removed = 0
    `)
	if err != nil {
		panic(err)
	}
	defer stmtRemove.Close()

	// --- Main Loop ---
	for job := range w.opCh {
		switch job.Type {

		case OpWriteEvent:
			e := job.Event
			_, err := stmtEvent.Exec(
				e.ID, e.RoomID, e.UserID, e.EntityID, e.Op, e.Payload, e.CreatedAt,
			)
			if err != nil {
				fmt.Printf("DB Error (Event): %v\n", err)
			}

		case OpDomUpsert:
			d := job.Dom
			_, err := stmtDom.Exec(
				d.ID, d.RoomID, d.UserID, d.Kind,
				d.Transform.X, d.Transform.Y, d.Transform.Rot, d.Transform.W, d.Transform.H,
				d.CreatedAt, d.UpdatedAt,
			)
			if err != nil {
				fmt.Printf("DB Error (Dom Upsert): %v\n", err)
			}

		case OpDomRemove:
			_, err := stmtRemove.Exec(
				time.Now().UnixMilli(),
				job.RemoveID,
				job.RemoveRoomID,
			)
			if err != nil {
				fmt.Printf("DB Error (Dom Remove): %v\n", err)
			}
		}
	}
}

// --- Public Write Methods ---

func WriteEvent(e config.Event) {
	if W == nil {
		return
	}
	select {
	case W.opCh <- DbJob{Type: OpWriteEvent, Event: e}:
	default:
		// channel full
	}
}

func WriteDom(e config.DomEvent) {
	if W == nil {
		return
	}
	// fmt.Println("ch writedom")
	select {
	case W.opCh <- DbJob{Type: OpDomUpsert, Dom: e}:
	default:
		fmt.Println("fail ch writeDom")
	}
}

func RemoveDom(id, roomId string) {
	if W == nil {
		return
	}
	// Now asynchronous via channel
	select {
	case W.opCh <- DbJob{Type: OpDomRemove, RemoveID: id, RemoveRoomID: roomId}:
	default:
		fmt.Println("fail ch removeDom")
	}
}

// --- Read Methods (Unchanged, safe for concurrent read) ---

func GetEvent(roomID string, id string) ([]config.Event, error) {
	// ... (Same as your original code)
	// Need to include the body here if you want a complete file copy-paste
	// I will assume you keep the original implementation here as it was correct.
	rows, err := W.db.Query(`
        SELECT id, room_id, user_id, entity_id, op, payload, created_at
        FROM events
        WHERE room_id = ? AND id > ? AND op = 'stroke-add'
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
			&e.ID, &e.RoomID, &e.UserID, &e.EntityID, &e.Op, &e.Payload, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	return events, nil
}

func GetActiveDomObjects(roomID string) ([]config.DomObjectNetwork, error) {
	// ... (Same as your original code)
	rows, err := W.db.Query(`
        SELECT
            id, kind, x, y, rot, w, h
        FROM dom_objects
        WHERE room_id = ?
        AND is_removed = 0
    `, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []config.DomObjectNetwork

	for rows.Next() {
		var dom config.DomObjectNetwork
		var t config.Transform

		err := rows.Scan(
			&dom.ID, &dom.Kind,
			&t.X, &t.Y, &t.Rot, &t.W, &t.H,
		)
		if err != nil {
			return nil, err
		}

		dom.Transform = t
		result = append(result, dom)
	}

	return result, nil
}
