package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Tk21111/whiteboard_server/auth"
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

		userId := auth.RequireUserId(r)
		if userId == "" {
			http.Error(w, "no userid", http.StatusForbidden)
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
