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
	OpDomCreate = iota
	OpDomTransform
	OpDomPayload
	OpDomRemove
	OpWriteEvent
	OpRoomCreate
	OpRoomEditUser
	OpUser
	OpLayerCreate
)

type DbJob struct {
	Type         int
	Event        config.Event
	Dom          config.DomEvent
	RemoveID     string
	RemoveRoomID string
	Room         config.RoomEvent
	User         config.UserEvent
	Layer        config.LayerEvent
}

type Writer struct {
	db   *sql.DB
	opCh chan DbJob
}

var (
	W *Writer
)

func NewWriter(dbPath string) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}

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
			layer INTEGER NOT NULL ,
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
			layer INTEGET NOT NULL ,
			payload TEXT NOT NULL DEFAULT "",
            is_removed INTEGER NOT NULL DEFAULT 0,

            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL
        );
    `)
	if err != nil {
		panic(err)
	}

	//room
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS rooms (
			room_id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			public INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		);
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS layers (
		layer_index INTEGER NOT NULL,
		room_id TEXT NOT NULL,
		owner_id TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT "",
		public INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,

		PRIMARY KEY (room_id, layer_index)
	);
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users_layers (
		room_id TEXT NOT NULL,
		layer_index INTEGER NOT NULL,
		user_id TEXT NOT NULL,

		PRIMARY KEY (room_id, layer_index, user_id),
		FOREIGN KEY (room_id, layer_index)
			REFERENCES layers(room_id, layer_index)
			ON DELETE CASCADE
	);
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_users_layers_user
		ON users_layers(user_id);
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users_data (
		user_id TEXT PRIMARY KEY,
		role INTEGER NOT NULL DEFAULT 0,

		name TEXT NOT NULL DEFAULT "",
		given_name TEXT NOT NULL DEFAULT "",
		email TEXT NOT NULL DEFAULT "",

		created_at INTEGER NOT NULL
	);
    `)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS users_rooms (
			user_id TEXT NOT NULL,
			room_id TEXT NOT NULL,
			role INTEGER NOT NULL DEFAULT 0,
			joined_at INTEGER NOT NULL,
			PRIMARY KEY (user_id, room_id)
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
		db:   db,
		opCh: make(chan DbJob, 10000),
	}

	go W.writerLoop()
}

func (w *Writer) writerLoop() {
	// 1. Prepare Event Statement
	stmtEvent, err := w.db.Prepare(`
        INSERT INTO events
        (id, room_id, user_id, entity_id, op, payload, layer,  created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7 , $8)
    `)
	if err != nil {
		panic(err)
	}
	defer stmtEvent.Close()

	// 2. Prepare Dom Upsert Statement
	stmtDomCreate, err := w.db.Prepare(`
		INSERT INTO dom_objects
		(
			id, room_id, user_id, kind,
			x, y, rot, w, h,
			payload, layer ,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? , ?)
	`)
	if err != nil {
		panic(err)
	}
	defer stmtDomCreate.Close()

	stmtDomTransform, err := w.db.Prepare(`
		UPDATE dom_objects
		SET
			x = ?,
			y = ?,
			rot = ?,
			w = ?,
			h = ?,
			updated_at = ?
		WHERE
			id = ?
			AND room_id = ?
			AND is_removed = 0
	`)
	if err != nil {
		panic(err)
	}
	defer stmtDomTransform.Close()

	stmtDomPayload, err := w.db.Prepare(`
		UPDATE dom_objects
		SET
			payload = ?,
			updated_at = ?
		WHERE
			id = ?
			AND room_id = ?
			AND is_removed = 0
	`)
	if err != nil {
		panic(err)
	}
	defer stmtDomPayload.Close()

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

	stmtEditRoom, err := w.db.Prepare(`
		INSERT INTO users_rooms (user_id, room_id, role, joined_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, room_id)
		DO UPDATE SET
			role = excluded.role
	`)
	if err != nil {
		panic(err)
	}
	defer stmtEditRoom.Close()

	stmtUser, err := w.db.Prepare(`
		INSERT INTO users_data (
			user_id,
			role,
			name,
			given_name,
			email,
			created_at
		)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id)
		DO UPDATE SET
			role = excluded.role,
			name = excluded.name,
			given_name = excluded.given_name,
			email = excluded.email
	`)

	if err != nil {
		panic(err)
	}
	defer stmtUser.Close()

	// --- Main Loop ---
	for job := range w.opCh {
		switch job.Type {

		case OpWriteEvent:
			e := job.Event
			_, err := stmtEvent.Exec(
				e.ID, e.RoomID, e.UserID, e.EntityID,
				e.Op, e.Payload, e.LayerIndex, e.CreatedAt,
			)
			if err != nil {
				fmt.Printf("DB Error (Event): %v\n", err)
			}

		case OpDomCreate:
			d := job.Dom
			_, err := stmtDomCreate.Exec(
				d.ID, d.RoomID, d.UserID, d.Kind,
				d.Transform.X, d.Transform.Y,
				d.Transform.Rot, d.Transform.W, d.Transform.H,
				d.Payload, d.LayerIndex,
				d.CreatedAt, d.UpdatedAt,
			)
			if err != nil {
				fmt.Printf("DB Error (Dom Create): %v\n", err)
			}

		case OpDomTransform:
			d := job.Dom
			_, err := stmtDomTransform.Exec(
				d.Transform.X, d.Transform.Y,
				d.Transform.Rot, d.Transform.W, d.Transform.H,
				d.UpdatedAt,
				d.ID, d.RoomID,
			)
			if err != nil {
				fmt.Printf("DB Error (Dom Transform): %v\n", err)
			}

		case OpDomPayload:
			d := job.Dom
			_, err := stmtDomPayload.Exec(
				d.Payload,
				d.UpdatedAt,
				d.ID, d.RoomID,
			)
			if err != nil {
				fmt.Printf("DB Error (Dom Payload): %v\n", err)
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
		case OpRoomCreate:
			j := job.Room

			tx, err := w.db.Begin()
			if err != nil {
				break
			}

			_, err = tx.Exec(
				`INSERT INTO rooms (room_id, owner_id, public , created_at)
		 		VALUES (?, ?, ?, ?)`,
				j.RoomID, j.UserID, j.Public, j.Now,
			)
			if err != nil {
				tx.Rollback()
				fmt.Println("room create")
				fmt.Println(err)
				break
			}

			_, err = tx.Exec(
				`INSERT INTO users_rooms (user_id, room_id, role, joined_at)
		 		VALUES (?, ?, 3, ?)`,
				j.UserID, j.RoomID, j.Now,
			)
			if err != nil {
				tx.Rollback()
				break
			}

			_, err = tx.Exec(`
					INSERT INTO layers (
						room_id,
						layer_index,
						owner_id,
						name,
						public,
						created_at
					) VALUES (?, 0, ?, 'Base Layer', 1, ?)
				`,
				j.RoomID,
				j.UserID,
				j.Now,
			)
			if err != nil {
				tx.Rollback()
				break
			}

			err = tx.Commit()
		case OpRoomEditUser:
			j := job.Room
			_, err := stmtEditRoom.Exec(
				j.UserID,
				j.RoomID,
				int(j.Role),
				time.Now().UnixMilli(),
			)
			if err != nil {
				fmt.Printf("DB Error (Join Room): %v\n", err)
			}
		case OpUser:
			j := job.User
			_, err := stmtUser.Exec(
				j.UserID,
				int(j.Role),
				j.Name,
				j.GivenName,
				j.Email,
				j.Created_at,
			)
			if err != nil {
				fmt.Printf("DB Error (Edit User): %v\n", err)
			}
		case OpLayerCreate:
			j := job.Layer

			tx, err := w.db.Begin()
			if err != nil {
				j.Result <- err
				break
			}

			var nextLayer int64 // ← Change to int64
			err = tx.QueryRow(`
		SELECT COALESCE(MAX(layer_index), -1) + 1
		FROM layers
		WHERE room_id = ?
	`, j.RoomID).Scan(&nextLayer)

			if err != nil {
				tx.Rollback()
				j.Result <- err
				break
			}

			_, err = tx.Exec(`
		INSERT INTO layers (
			room_id,
			layer_index,
			owner_id,
			name,
			public,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
				j.RoomID,
				nextLayer,
				j.UserID,
				j.Name,
				j.Public,
				j.Now,
			)

			if err != nil {
				tx.Rollback()
				j.Result <- err
				break
			}

			// Owner always has access
			_, err = tx.Exec(`
		INSERT INTO users_layers (room_id, layer_index, user_id)
		VALUES (?, ?, ?)
	`,
				j.RoomID,
				nextLayer,
				j.UserID,
			)

			if err != nil {
				tx.Rollback()
				j.Result <- err
				break
			}

			err = tx.Commit()
			j.Result <- err

			if err == nil {
				j.LayerIndex <- nextLayer // ← Send back the created layer index
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

func WriteDom(e config.DomEvent, op int) {
	if W == nil {
		return
	}
	// fmt.Println("ch writedom")
	select {
	case W.opCh <- DbJob{Type: op, Dom: e}:
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

// crate and join
func CreateRoom(roomId, userId string, public int8, mainArea int64, subArea int64) error {
	if W == nil {
		return fmt.Errorf("writer not initialized")
	}

	W.opCh <- DbJob{
		Type: OpRoomCreate,
		Room: config.RoomEvent{
			RoomID: roomId,
			UserID: userId,
			Public: public,
			Now:    time.Now().UnixMilli(),
		},
	}

	return nil
}

func JoinRoom(roomId, userId string, role config.Role) error {
	if W == nil {
		return fmt.Errorf("writer not initialized")
	}

	select {
	case W.opCh <- DbJob{
		Type: OpRoomEditUser,
		Room: config.RoomEvent{
			RoomID: roomId,
			UserID: userId,
			Role:   role,
		},
	}:
	default:
		fmt.Println("fail ch joinRoom")
	}

	return nil
}

func CreateUser(
	userId string,
	role config.Role,
	name string,
	givenName string,
	email string,
) error {
	if W == nil {
		return fmt.Errorf("writer not initialized")
	}

	select {
	case W.opCh <- DbJob{
		Type: OpUser,
		User: config.UserEvent{
			UserID:     userId,
			Role:       role,
			Name:       name,
			GivenName:  givenName,
			Email:      email,
			Created_at: time.Now().Unix(),
		},
	}:
	default:
		fmt.Println("fail ch createUser")
	}

	return nil
}

func CreateLayer(roomId, userId, name string, public int) (int64, error) {
	if W == nil {
		return -1, fmt.Errorf("writer not initialized")
	}

	result := make(chan error, 1)
	layerIndex := make(chan int64, 1) // ← Add channel for layer index

	W.opCh <- DbJob{
		Type: OpLayerCreate,
		Layer: config.LayerEvent{
			RoomID:     roomId,
			UserID:     userId,
			Name:       name,
			Public:     public,
			Now:        time.Now().UnixMilli(),
			Result:     result,
			LayerIndex: layerIndex, // ← Pass the channel
		},
	}

	err := <-result
	if err != nil {
		return -1, err
	}

	return <-layerIndex, nil // ← Return the created layer index
}

// --- Read Methods (Unchanged, safe for concurrent read) ---

func GetEvent(roomID string, id string, layer int) ([]config.Event, error) {
	// ... (Same as your original code)
	// Need to include the body here if you want a complete file copy-paste
	// I will assume you keep the original implementation here as it was correct.
	rows, err := W.db.Query(`
        SELECT id, room_id, user_id, entity_id, op, payload, created_at
        FROM events
        WHERE room_id = ? AND id > ? AND op = 'stroke-add' AND layer = ?
        ORDER BY id ASC
    `, roomID, id, layer)
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

func GetActiveDomObjects(roomID string, layer int64) ([]config.DomObjectNetwork, error) {
	// ... (Same as your original code)
	rows, err := W.db.Query(`
        SELECT
            id, kind, x, y, rot, w, h , payload , layer
        FROM dom_objects
        WHERE room_id = ?
        AND is_removed = 0
		AND layer = ?
    `, roomID, layer)
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
			&t.X, &t.Y, &t.Rot, &t.W, &t.H, &dom.Payload, &dom.LayerIndex,
		)
		if err != nil {
			return nil, err
		}

		dom.Transform = t
		result = append(result, dom)
	}

	return result, nil
}

func GetMaxIdByRoom(roomID string) (int64, error) {
	var maxID int64

	err := W.db.QueryRow(`
		SELECT COALESCE(MAX(id), 0)
		FROM events
		WHERE room_id = ?
	`, roomID).Scan(&maxID)

	if err != nil {
		return 0, err
	}

	return maxID, nil
}

type ViewResult int

const (
	NotExist ViewResult = iota
	NoPerm
	Perm
)

func CheckRegister(userId string) (ViewResult, error) {
	var dummy int

	err := W.db.QueryRow(`
		SELECT 1
		FROM users_data
		WHERE user_id = ?
	`, userId).Scan(&dummy)

	if err == sql.ErrNoRows {
		return NotExist, nil
	}
	if err != nil {
		return NotExist, err
	}

	return Perm, nil
}

func CheckCanViewRoom(roomId string, userId string) (ViewResult, error) {
	var ownerID string
	var isPublic int

	err := W.db.QueryRow(`
		SELECT owner_id, public
		FROM rooms
		WHERE room_id = ?
	`, roomId).Scan(&ownerID, &isPublic)

	if err == sql.ErrNoRows {
		return NotExist, nil
	}
	if err != nil {
		return NoPerm, err
	}

	if ownerID == userId || isPublic == 1 {
		return Perm, nil
	}

	return NoPerm, nil
}

func CheckcanEditRoom(roomId string, userId string) (ViewResult, error) {
	var ownerID string
	var isPublic int

	err := W.db.QueryRow(`
		SELECT owner_id, public
		FROM rooms
		WHERE room_id = ?
	`, roomId).Scan(&ownerID, &isPublic)

	if err == sql.ErrNoRows {
		return NotExist, nil
	}
	if err != nil {
		return NoPerm, err
	}

	if ownerID == userId {
		return Perm, nil
	}

	return NoPerm, nil
}

func GetUserIDByEmail(email string) (string, error) {
	var userID string

	err := W.db.QueryRow(`
		SELECT user_id
		FROM users_data
		WHERE email = ?
	`, email).Scan(&userID)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return userID, nil
}

func GetUserRole(userId string) (int, error) {
	var role int

	err := W.db.QueryRow(`
		SELECT role
		FROM users_data
		WHERE user_id = ?
	`, userId).Scan(&role)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return role, nil
}

func GetUserRoomRole(roomId string, userId string) (int64, error) {
	var role int64

	err := W.db.QueryRow(`
		SELECT role
		FROM users_rooms
		WHERE user_id = ? AND room_id = ?
	`, userId, roomId).Scan(&role)

	if err == sql.ErrNoRows {
		return -1, nil
	}
	if err != nil {
		return -2, err
	}

	return role, nil
}

func CheckRoomExisted(roomId string) (bool, error) {
	var dummy string

	err := W.db.QueryRow(`
		SELECT 1
		FROM rooms
		WHERE room_id = ?
	`, roomId).Scan(&dummy)

	if err == sql.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

func GetAllUserInRoom(roomId string) ([]config.UserEvent, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if roomId != "" {
		// room-based role
		rows, err = W.db.Query(`
			SELECT
				u.user_id,
				ur.role,           -- room role
				u.name,
				u.given_name,
				u.email,
				u.created_at
			FROM users_data u
			INNER JOIN users_rooms ur ON u.user_id = ur.user_id
			WHERE ur.room_id = ?
		`, roomId)
	} else {
		// global role
		rows, err = W.db.Query(`
			SELECT
				user_id,
				role,              -- global role
				name,
				given_name,
				email,
				created_at
			FROM users_data
			WHERE role < 2
		`)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []config.UserEvent
	for rows.Next() {
		var u config.UserEvent
		var role int

		if err := rows.Scan(
			&u.UserID,
			&role,
			&u.Name,
			&u.GivenName,
			&u.Email,
			&u.Created_at,
		); err != nil {
			return nil, err
		}

		u.Role = config.Role(role)
		users = append(users, u)
	}

	return users, nil
}

func GetAllAreaWithPerm(roomId string, userId string) ([]config.Area, error) {
	var perms []config.Area

	rows, err := W.db.Query(`
        SELECT a.x, a.y, a.size
        FROM areas a
        LEFT JOIN users_area ua ON a.x = ua.x AND a.y = ua.y AND ua.user_id = ?
        WHERE a.room_id = ? 
        AND (a.public = 1 OR ua.user_id IS NOT NULL)
    `, userId, roomId)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p config.Area
		if err := rows.Scan(&p.X, &p.Y, &p.Size); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}

	return perms, nil
}

func CheckCanUseLayer(roomId string, layerIndex int64, userId string) (bool, error) {
	var dummy int

	err := W.db.QueryRow(`
		SELECT 1
		FROM layers l
		LEFT JOIN users_layers ul
			ON l.room_id = ul.room_id
			AND l.layer_index = ul.layer_index
			AND ul.user_id = ?
		WHERE l.room_id = ?
		  AND l.layer_index = ?
		  AND (l.public = 1 OR ul.user_id IS NOT NULL)
	`, userId, roomId, layerIndex).Scan(&dummy)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func GetLayerByUserId(userId string, roomId string) (int64, error) {
	var layerIndex int64

	err := W.db.QueryRow(`
		SELECT layer_index
		FROM users_layers
		WHERE room_id = ?
		  AND user_id = ?
		LIMIT 1
	`, roomId, userId, userId).Scan(&layerIndex)

	if err == sql.ErrNoRows {
		return -1, nil // User doesn't have a private layer yet
	}
	if err != nil {
		return -1, err
	}

	return layerIndex, nil
}
func GetAllRooms(userId string) ([]config.RoomEvent, error) {
	rows, err := W.db.Query(`
		SELECT r.room_id, r.owner_id, r.public
		FROM rooms r
		LEFT JOIN users_rooms ur ON r.room_id = ur.room_id AND ur.user_id = ?
		WHERE r.public = 1 OR r.owner_id = ? OR ur.user_id IS NOT NULL
		ORDER BY r.created_at DESC
	`, userId, userId)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []config.RoomEvent
	for rows.Next() {
		var r config.RoomEvent
		if err := rows.Scan(&r.RoomID, &r.UserID, &r.Public); err != nil {
			return nil, err
		}
		rooms = append(rooms, r)
	}
	return rooms, nil
}
