package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Tk21111/whiteboard_server/auth"
	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/db"
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
			Bucket:      aws.String(os.Getenv("R2_BUCKET")),
			Key:         aws.String(objectKey),
			ContentType: aws.String(req.MimeType),
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

		events, err := db.GetEvent(roomID, from)
		if err != nil {
			http.Error(w, "fail to get event replay", http.StatusInternalServerError)
			return
		}

		replay := make([]config.ServerMsg, 0, len(events))

		for _, e := range events {

			var payload config.NetworkMsg = config.NetworkMsg{
				ID:        e.EntityID,
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

			// fmt.Println("%#v\n", replay)
		}

		dom_objects, err := db.GetActiveDomObjects(roomID)
		if err != nil {
			http.Error(w, "fail to get dom replay", http.StatusInternalServerError)
			return
		}

		for _, d := range dom_objects {

			var payload config.NetworkMsg = config.NetworkMsg{
				ID:        d.ID,
				Operation: "dom-add",
				DomObject: &d,
			}

			replay = append(replay, config.ServerMsg{
				Clock:   0,
				Payload: payload,
			})

			// fmt.Println("%#v\n", replay)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(replay)
	}
}
