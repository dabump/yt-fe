package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		log.Println("Error: yt-dlp not found. Please install yt-dlp first.")
		os.Exit(1)
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("Error: ffmpeg not found. Please install ffmpeg first.")
		os.Exit(1)
	}

	app, err := NewApp(loadConfig())
	if err != nil {
		log.Fatalf("Failed to start app: %v", err)
	}

	if err := app.ensureDirectories(); err != nil {
		log.Fatalf("Failed to create directories: %v", err)
	}

	app.generateMissingThumbnails()

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.indexHandler)
	mux.HandleFunc("/download", app.downloadHandler)
	mux.HandleFunc("/thumbnails/", app.thumbnailHandler)
	mux.HandleFunc("/video/", app.videoHandler)
	mux.HandleFunc("/delete/", app.deleteHandler)
	mux.HandleFunc("/api/status", app.apiStatusHandler)

	go app.processDownloadQueue()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	url := fmt.Sprintf("http://localhost:%s", port)
	fmt.Printf("yt-fe server starting on %s\n", url)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}
