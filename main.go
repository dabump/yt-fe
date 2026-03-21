package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Video struct {
	Name      string
	Filename  string
	Thumbnail string
	Title     string
	Date      time.Time
}

type VideoMetadata struct {
	Title        string `json:"title"`
	VideoID      string `json:"video_id"`
	URL          string `json:"url"`
	DownloadedAt string `json:"downloaded_at"`
}

type PageData struct {
	Videos      []Video
	Error       string
	Success     string
	Downloading bool
	QueueCount  int
}

type DownloadJob struct {
	URL      string
	VideoID  string
	Filename string
}

type DownloadStatus struct {
	Current    *DownloadJob
	Queue      []DownloadJob
	Processing bool
	Progress   float64
}

type Config struct {
	VideoDir      string `yaml:"video_dir"`
	ThumbnailsDir string `yaml:"thumbnails_dir"`
	MetadataDir   string `yaml:"metadata_dir"`
	StaticDir     string `yaml:"static_dir"`
}

var config Config

func loadConfig() Config {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return Config{
			VideoDir:      "video",
			ThumbnailsDir: "thumbnails",
			MetadataDir:   "metadata",
			StaticDir:     "static",
		}
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{
			VideoDir:      "video",
			ThumbnailsDir: "thumbnails",
			MetadataDir:   "metadata",
			StaticDir:     "static",
		}
	}
	return cfg
}

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

	os.MkdirAll(config.VideoDir, 0755)
	os.MkdirAll(config.ThumbnailsDir, 0755)
	os.MkdirAll(config.MetadataDir, 0755)
	os.MkdirAll(config.StaticDir, 0755)

	generateMissingThumbnails()

	fs := http.FileServer(http.Dir(config.StaticDir))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

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

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleForm(w, r)
		return
	}
	serveIndex(w, r, "", "")
}

func handleForm(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	if url == "" {
		serveIndex(w, r, "", "Please provide a YouTube URL")
		return
	}

	url = cleanYouTubeURL(url)

	videoID := extractVideoID(url)
	if videoID == "" {
		serveIndex(w, r, "", "Invalid YouTube URL. Please provide a valid YouTube video link (e.g., https://www.youtube.com/watch?v=VIDEO_ID)")
		return
	}

	metadata, err := getVideoMetadata(url)
	if err != nil {
		serveIndex(w, r, "", err.Error())
		return
	}

	filename := fmt.Sprintf("%s.mp4", uuid.New().String())

	queueMutex.Lock()
	downloadQueue = append(downloadQueue, DownloadJob{
		URL:      url,
		VideoID:  videoID,
		Filename: filename,
	})
	downloadStatus.Queue = downloadQueue
	queueMutex.Unlock()

	_ = metadata
	serveIndex(w, r, "Video added to download queue!", "")
}

func getVideoMetadata(url string) (VideoMetadata, error) {
	cmd := exec.Command("yt-dlp", "--dump-json", "--no-download", "--no-warnings", url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "is not a valid URL") || strings.Contains(errMsg, "Unsupported URL") {
			return VideoMetadata{}, fmt.Errorf("invalid YouTube URL: %s", url)
		}
		if strings.Contains(errMsg, "Video unavailable") || strings.Contains(errMsg, "is unavailable") {
			return VideoMetadata{}, fmt.Errorf("this YouTube video is unavailable or has been removed")
		}
		if strings.Contains(errMsg, "Unable to extract") || strings.Contains(errMsg, "ERROR") {
			return VideoMetadata{}, fmt.Errorf("could not fetch video information. The video may not exist or YouTube may be blocking the request")
		}
		return VideoMetadata{}, fmt.Errorf("failed to get video information: %s", errMsg)
	}

	var data struct {
		Title     string `json:"title"`
		DisplayID string `json:"display_id"`
		ID        string `json:"id"`
	}

	jsonStr := strings.TrimSpace(string(output))
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return VideoMetadata{}, fmt.Errorf("failed to parse video information")
	}

	videoID := data.DisplayID
	if videoID == "" {
		videoID = data.ID
	}

	return VideoMetadata{
		Title:        data.Title,
		VideoID:      videoID,
		URL:          url,
		DownloadedAt: time.Now().Format(time.RFC3339),
	}, nil
}

func saveMetadata(filename string, metadata VideoMetadata) {
	absPath, _ := filepath.Abs(config.MetadataDir)
	metadataPath := filepath.Join(absPath, strings.TrimSuffix(filename, ".mp4")+".json")
	data, _ := json.MarshalIndent(metadata, "", "  ")
	os.WriteFile(metadataPath, data, 0644)
}

func loadMetadata(filename string) VideoMetadata {
	absPath, _ := filepath.Abs(config.MetadataDir)
	metadataPath := filepath.Join(absPath, strings.TrimSuffix(filename, ".mp4")+".json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return VideoMetadata{}
	}
	var metadata VideoMetadata
	json.Unmarshal(data, &metadata)
	return metadata
}

func convertToMP4(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-c:v", "libx264", "-c:a", "aac", "-strict", "experimental", outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func serveIndex(w http.ResponseWriter, r *http.Request, success, errMsg string) {
	videos, err := getVideos()
	if err != nil {
		errMsg = fmt.Sprintf("Error reading videos: %v", err)
	}

	data := PageData{
		Videos:  videos,
		Error:   errMsg,
		Success: success,
	}

	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, data)
}

func getVideos() ([]Video, error) {
	entries, err := os.ReadDir(config.VideoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Video{}, nil
		}
		return nil, err
	}

	var videos []Video
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".mp4") {
			continue
		}
		thumbName := strings.TrimSuffix(entry.Name(), ".mp4") + ".jpg"
		thumbPath := filepath.Join(config.ThumbnailsDir, thumbName)
		if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
			generateThumbnail(filepath.Join(config.VideoDir, entry.Name()), entry.Name())
		}
		info, _ := entry.Info()
		metadata := loadMetadata(entry.Name())
		title := metadata.Title
		if title == "" {
			title = strings.TrimSuffix(entry.Name(), ".mp4")
		}
		videos = append(videos, Video{
			Name:      strings.TrimSuffix(entry.Name(), ".mp4"),
			Filename:  entry.Name(),
			Thumbnail: "/thumbnails/" + thumbName,
			Title:     title,
			Date:      info.ModTime(),
		})
	}

	sort.Slice(videos, func(i, j int) bool {
		return videos[i].Date.After(videos[j].Date)
	})

	return videos, nil
}

func extractVideoID(url string) string {
	parts := strings.Split(url, "v=")
	if len(parts) > 1 {
		id := strings.Split(parts[1], "&")[0]
		if len(id) == 11 {
			return id
		}
	}
	parts = strings.Split(url, "youtu.be/")
	if len(parts) > 1 {
		id := strings.Split(parts[1], "?")[0]
		if len(id) == 11 {
			return id
		}
	}
	return ""
}

func cleanYouTubeURL(rawURL string) string {
	videoID := extractVideoID(rawURL)
	if videoID == "" {
		return rawURL
	}
	if strings.Contains(rawURL, "youtu.be/") {
		return "https://youtu.be/" + videoID
	}
	return "https://www.youtube.com/watch?v=" + videoID
}

func generateThumbnail(videoPath, filename string) {
	absPath, _ := filepath.Abs(config.ThumbnailsDir)
	os.MkdirAll(absPath, 0755)
	thumbName := strings.TrimSuffix(filename, ".mp4") + ".jpg"
	thumbPath := filepath.Join(absPath, thumbName)
	fmt.Printf("Generating thumbnail: ffmpeg -y -i %s -ss 00:00:01 -vframes 1 -q:v 2 %s\n", videoPath, thumbPath)
	cmd := exec.Command("ffmpeg", "-y", "-i", videoPath, "-ss", "00:00:01", "-vframes", "1", "-q:v", "2", thumbPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to generate thumbnail: %v\n", err)
	}
}

func generateMissingThumbnails() {
	entries, err := os.ReadDir(config.VideoDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".mp4") {
			continue
		}
		thumbName := strings.TrimSuffix(entry.Name(), ".mp4") + ".jpg"
		thumbPath := filepath.Join(config.ThumbnailsDir, thumbName)
		if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
			videoPath := filepath.Join(config.VideoDir, entry.Name())
			fmt.Printf("Generating thumbnail for %s...\n", entry.Name())
			generateThumbnail(videoPath, entry.Name())
		}
	}
}

func thumbnailHandler(w http.ResponseWriter, r *http.Request) {
	thumbName := filepath.Base(r.URL.Path)
	thumbPath := filepath.Join(config.ThumbnailsDir, thumbName)
	file, err := os.Open(thumbPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	io.Copy(w, file)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	handleForm(w, r)
}

func videoHandler(w http.ResponseWriter, r *http.Request) {
	videoName := filepath.Base(r.URL.Path)
	videoPath := filepath.Join(config.VideoDir, videoName)
	file, err := os.Open(videoPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "video/mp4")
	io.Copy(w, file)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	videoName := filepath.Base(r.URL.Path)
	videoPath := filepath.Join(config.VideoDir, videoName)
	thumbName := strings.TrimSuffix(videoName, ".mp4") + ".jpg"
	thumbPath := filepath.Join(config.ThumbnailsDir, thumbName)
	metadataName := strings.TrimSuffix(videoName, ".mp4") + ".json"
	metadataPath := filepath.Join(config.MetadataDir, metadataName)
	os.Remove(videoPath)
	os.Remove(thumbPath)
	os.Remove(metadataPath)
}

func processDownloadQueue() {
	for {
		queueMutex.Lock()
		if len(downloadQueue) == 0 {
			downloadStatus.Processing = false
			downloadStatus.Current = nil
			downloadStatus.Queue = []DownloadJob{}
			queueMutex.Unlock()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		job := downloadQueue[0]
		downloadQueue = downloadQueue[1:]
		downloadStatus.Queue = downloadQueue
		downloadStatus.Processing = true
		downloadStatus.Current = &job
		downloadStatus.Progress = 0
		queueMutex.Unlock()

		fmt.Printf("Processing download: %s\n", job.URL)

		metadata, err := getVideoMetadata(job.URL)
		if err != nil {
			fmt.Printf("Failed to get metadata for %s: %v\n", job.URL, err)
			continue
		}

		absPath, _ := filepath.Abs(config.VideoDir)
		videoPath := filepath.Join(absPath, job.Filename)
		tempPath := videoPath + ".temp"

		outputTemplate := tempPath + ".%(ext)s"
		cmd := exec.Command("yt-dlp", "-f", "bestvideo+bestaudio/best", "-o", outputTemplate, job.URL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			fmt.Printf("Failed to download %s: %v\n", job.URL, err)
			continue
		}

		downloadMutex.Lock()
		downloadStatus.Progress = 50
		downloadMutex.Unlock()

		downloadedFile := tempPath + ".mkv"
		if _, err := os.Stat(downloadedFile); os.IsNotExist(err) {
			files, _ := os.ReadDir(absPath)
			for _, f := range files {
				if strings.HasPrefix(f.Name(), job.Filename+".temp.") {
					downloadedFile = filepath.Join(absPath, f.Name())
					break
				}
			}
		}

		if err := convertToMP4(downloadedFile, videoPath); err != nil {
			fmt.Printf("Failed to convert %s: %v\n", job.URL, err)
			os.Remove(downloadedFile)
			continue
		}
		os.Remove(downloadedFile)

		downloadMutex.Lock()
		downloadStatus.Progress = 75
		downloadMutex.Unlock()

		saveMetadata(job.Filename, metadata)
		generateThumbnail(videoPath, job.Filename)

		downloadMutex.Lock()
		downloadStatus.Progress = 100
		downloadMutex.Unlock()

		fmt.Printf("Download complete: %s\n", job.Filename)

		time.Sleep(100 * time.Millisecond)
	}
}

var downloadMutex sync.Mutex

func apiStatusHandler(w http.ResponseWriter, r *http.Request) {
	queueMutex.Lock()
	status := struct {
		Processing bool          `json:"processing"`
		Current    *DownloadJob  `json:"current"`
		Queue      []DownloadJob `json:"queue"`
		Progress   float64       `json:"progress"`
	}{
		Processing: downloadStatus.Processing,
		Current:    downloadStatus.Current,
		Queue:      downloadStatus.Queue,
		Progress:   downloadStatus.Progress,
	}
	queueMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
