package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

var (
	downloadQueue   []DownloadJob
	downloadStatus  DownloadStatus
	queueMutex      sync.Mutex
	currentDownload *DownloadJob
)

func main() {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		fmt.Println("Error: yt-dlp not found. Please install yt-dlp first.")
		os.Exit(1)
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Println("Error: ffmpeg not found. Please install ffmpeg first.")
		os.Exit(1)
	}

	config = loadConfig()

	os.MkdirAll(config.VideoDir, 0o755)
	os.MkdirAll(config.ThumbnailsDir, 0o755)
	os.MkdirAll(config.MetadataDir, 0o755)

	generateMissingThumbnails()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/thumbnails/", thumbnailHandler)
	http.HandleFunc("/video/", videoHandler)
	http.HandleFunc("/delete/", deleteHandler)
	http.HandleFunc("/api/status", apiStatusHandler)

	go processDownloadQueue()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	url := fmt.Sprintf("http://localhost:%s", port)
	fmt.Printf("yt-fe server starting on %s\n", url)

	go func() {
		time.Sleep(100 * time.Millisecond)
		openBrowser(url)
	}()

	http.ListenAndServe(":"+port, nil)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Run()
}
