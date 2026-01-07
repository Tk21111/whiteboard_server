package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type SignRequest struct {
	Size     int64  `json:"size"`
	RoomId   string `json:"roomId"`
	MimeType string `json:"mimeType"`
	Name     string `json:"name"`
}

func UploadHandler(client *s3.PresignClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			http.Error(w, "Use POST", http.StatusMethodNotAllowed)
			return
		}

		var req SignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		const maxLimit = 10 * 1024 * 1024
		if req.Size > maxLimit {
			http.Error(w, "file excess max size", http.StatusForbidden)
		}

		objectKey := fmt.Sprintf("rooms/%s/%s", req.RoomId, req.Name)

		presignedReq, err := client.PresignPutObject(r.Context(), &s3.PutObjectInput{
			Bucket:      aws.String(os.Getenv("whiteboard-media")),
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

func getObject(client *s3.PresignClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		objectKey := r.URL.Query().Get("key")
		if objectKey == "" {
			http.Error(w, "key require", http.StatusBadRequest)
		}

		//TODO - implement this
		// if !userHasAccessToKey(r, objectKey) {
		// 	http.Error(w, "Unauthorized", http.StatusForbidden)
		// 	return
		// }

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

		// 4. Return the URL
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"download_url": presignedReq.URL,
		})
	}
}
