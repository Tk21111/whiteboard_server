package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/Tk21111/whiteboard_server/api"
	"github.com/Tk21111/whiteboard_server/auth"
	"github.com/Tk21111/whiteboard_server/db"
	"github.com/Tk21111/whiteboard_server/middleware"
	"github.com/Tk21111/whiteboard_server/ws"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"
)

func main() {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
		return
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				os.Getenv("R2_KEY"),
				os.Getenv("R2_SECRET"),
				"",
			),
		),
		config.WithRegion("auto"),
	)
	if err != nil {
		panic(err)
	}

	db.NewWriter("./db/sql/events.db")

	go ws.StartStrokeTTLGC()

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(os.Getenv("R2_ENDPOINT"))
	})

	http.HandleFunc("/start-test", startTestHandler)

	presignClient := s3.NewPresignClient(client)
	http.HandleFunc("/ws", ws.HandleWS)

	cookieHandler := middleware.CORSMiddleware(auth.HandleAuthAsset())
	http.Handle("/cookie", cookieHandler)

	replayHandler := middleware.CORSMiddleware(api.GetReplay())
	http.Handle("/get-replay", replayHandler)

	validToken := middleware.CORSMiddleware(auth.HandleValidate())
	http.Handle("/check-valid", validToken)

	uploadHandler :=
		middleware.CORSMiddleware(
			middleware.AuthMiddleware(
				api.UploadHandler(presignClient),
			),
		)
	http.Handle("/upload", uploadHandler)

	getHandler :=
		middleware.CORSMiddleware(
			middleware.RequireSession(
				api.GetObject(presignClient),
			),
		)
	http.Handle("/get", getHandler)

	log.Println("WS running :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func startTestHandler(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("roomId")
	if roomID == "" {
		roomID = "test"
	}

	strokes := 100_000
	updates := 30

	go ws.BurnRoom(&ws.H, roomID, strokes, updates)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("🔥 load test started\n"))
}
