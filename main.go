package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	// --------------------------------------------------
	// ENV (local dev OK, Fly ignores if missing)
	// --------------------------------------------------
	_ = godotenv.Load(".env")

	// --------------------------------------------------
	// AWS R2 / S3 CONFIG
	// --------------------------------------------------
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
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
		log.Fatal("failed to load aws config:", err)
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(os.Getenv("R2_ENDPOINT"))
	})
	presignClient := s3.NewPresignClient(s3Client)

	// --------------------------------------------------
	// DB / BACKGROUND JOBS
	// --------------------------------------------------
	err = os.MkdirAll("./data", 0755)
	if err != nil {
		log.Fatal("failed to create db directory:", err)
	}
	db.NewWriter("./data/events.db")
	go ws.StartStrokeTTLGC()

	// --------------------------------------------------
	// ROUTES
	// --------------------------------------------------
	mux := http.NewServeMux()

	// --- testing
	mux.HandleFunc("/start-test", startTestHandler)

	// --- websocket
	mux.HandleFunc("/ws", ws.HandleWS)

	// --- auth / cookies
	mux.Handle("/cookie",
		auth.HandleAuthAsset(),
	)

	mux.Handle("/check-valid",
		auth.HandleValidate(),
	)

	// --- replay
	mux.Handle("/get-replay",
		middleware.RequireSession(api.GetReplay()),
	)

	// --- upload
	mux.Handle("/upload",
		middleware.AuthMiddleware(
			api.UploadHandler(presignClient),
		),
	)

	// --- get object
	mux.Handle("/get",
		middleware.RequireSession(
			api.GetObject(presignClient),
		),
	)

	// --- room admin
	mux.Handle("/add-user",
		middleware.RequireSession(api.OwnerAddUser()),
	)

	mux.Handle("/get-users",
		middleware.RequireSession(
			middleware.RequireRole(api.GetAllUserInRoom(), 3),
		),
	)

	// --- admin panel
	adminFS := http.FileServer(http.Dir("./web"))
	mux.Handle(
		"/admin/",
		middleware.RequireSession(
			middleware.RequireRole(
				http.StripPrefix("/admin/", adminFS),
				3,
			),
		),
	)

	// --------------------------------------------------
	// SERVER (Fly.io compatible)
	// --------------------------------------------------
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := middleware.CORSMiddleware(mux)
	addr := "0.0.0.0:" + port
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	log.Println("🚀 Whiteboard server running on", addr)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error:", err)
		}
	}()

	// --------------------------------------------------
	// GRACEFUL SHUTDOWN (Fly sends SIGTERM)
	// --------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Println("shutdown error:", err)
	}

	log.Println("✅ server stopped cleanly")
}

// --------------------------------------------------
// LOAD TEST
// --------------------------------------------------
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
