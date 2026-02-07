package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tk21111/whiteboard_server/auth"
	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/db"
	"github.com/Tk21111/whiteboard_server/ws"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type SignRequest struct {
	Size     int64  `json:"size"`
	RoomId   string `json:"roomId"`
	MimeType string `json:"mimeType"`
}

func UploadHandler(client *s3.PresignClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		userId, err := auth.RequireUserId(r.Context())
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Use POST", http.StatusMethodNotAllowed)
			return
		}

		var req SignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		fmt.Printf("%v\n", req.MimeType)
		if strings.HasPrefix(req.MimeType, "image/") {
			if req.Size > 10*1024*1024 {
				http.Error(w, "image too large", http.StatusForbidden)
				return
			}
		}

		if strings.HasPrefix(req.MimeType, "video/") {
			if req.Size > 200*1024*1024 {
				http.Error(w, "video too large", http.StatusForbidden)
				return
			}
		}

		objectKey := fmt.Sprintf("rooms/%s/%s-%d", req.RoomId, userId, time.Now().UnixNano())

		presignedReq, err := client.PresignPutObject(r.Context(), &s3.PutObjectInput{
			Bucket:             aws.String(os.Getenv("R2_BUCKET")),
			Key:                aws.String(objectKey),
			ContentType:        aws.String(req.MimeType),
			CacheControl:       aws.String("private, max-age=0"),
			ContentDisposition: aws.String("inline"),
		}, func(po *s3.PresignOptions) {
			po.Expires = 15 * time.Minute
		})

		if err != nil {
			http.Error(w, "signed fail", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode((map[string]string{
			"upload_url": presignedReq.URL,
			"key":        objectKey,
		}))
	}
}

func GetObject(client *s3.PresignClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		objectKey := r.URL.Query().Get("key")
		if objectKey == "" {
			http.Error(w, "key require", http.StatusBadRequest)
		}

		presignedReq, err := client.PresignGetObject(r.Context(), &s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("R2_BUCKET")),
			Key:    aws.String(objectKey),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = 1 * time.Hour
		})

		if err != nil {
			http.Error(w, "Failed to sign URL", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"download_url": presignedReq.URL,
		})
	}
}

func GetReplay() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		roomID := r.URL.Query().Get("roomId")
		from := r.URL.Query().Get("from")

		if roomID == "" {
			http.Error(w, "roomId required", http.StatusBadRequest)
			return
		}

		if from == "" {
			from = "0"
		}

		userId, err := auth.RequireUserId(r.Context())
		if err != nil {
			http.Error(w, "cannot get userId", 500)
			return
		}

		_, err = db.EnsureUserInRoom(roomID, userId)
		if err != nil {
			http.Error(w, "user don't have perm", 403)
			return
		}

		events, err := db.GetEvent(roomID, from)
		if err != nil {
			http.Error(w, "fail to get event replay", 500)
			return
		}

		replay := make([]config.ServerMsg, 0, len(events))

		for _, e := range events {

			payload := config.NetworkMsg{
				ID:        strconv.FormatInt(e.ID, 10),
				Operation: e.Op,
			}

			switch e.Op {
			case "stroke-add":
				var decoded config.StrokeObjectInterface
				if err := json.Unmarshal(e.Payload, &decoded); err != nil {
					continue
				}
				payload.Stroke = &decoded
			}

			replay = append(replay, config.ServerMsg{
				Clock:   e.ID,
				Payload: payload,
			})
		}

		doms, err := db.GetActiveDomObjects(roomID)
		if err != nil {
			http.Error(w, "fail to get dom replay", 500)
			return
		}

		for _, d := range doms {
			replay = append(replay, config.ServerMsg{
				Clock: 0,
				Payload: config.NetworkMsg{
					ID:        d.ID,
					Operation: "dom-add",
					DomObject: &d,
				},
			})
		}

		ws.StrokeBuffer.Mu.Lock()
		for _, d := range ws.StrokeBuffer.Buffer {
			if d.Meta.RoomID != roomID {
				continue
			}

			replay = append(replay, config.ServerMsg{
				Clock: 0,
				Payload: config.NetworkMsg{
					ID:        d.Stroke.ID,
					Operation: "stroke-start",
					Stroke:    d.Stroke,
				},
			})
		}
		ws.StrokeBuffer.Mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(replay)
	}
}

type AddUserReq struct {
	RoomID string `json:"roomId"`
	User   string `json:"user"` // email or userId
	Role   int    `json:"role"`
}

func OwnerAddUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerID := r.Context().Value(config.ContextUserIDKey).(string)

		var req AddUserReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		// permission check
		result, err := db.CheckcanEditRoom(req.RoomID, ownerID)
		if err != nil || !PermHelper(&result, w) {
			return
		}

		targetUserID := req.User
		if strings.Contains(req.User, "@") {
			uid, err := db.GetUserIDByEmail(req.User)
			if err != nil || uid == "" {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			targetUserID = uid
		}

		db.JoinRoom(req.RoomID, targetUserID, config.IntToRole(req.Role))
		w.WriteHeader(http.StatusOK)
	}
}

func GetAllUserInRoom() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		roomID := r.URL.Query().Get("roomId")
		users, err := db.GetAllUserInRoom(roomID)
		if err != nil {
			http.Error(w, "cannot get users", 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(users)

	}
}
