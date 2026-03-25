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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var downloadMutex sync.Mutex

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

	filename := fmt.Sprintf("%s.webm", uuid.New().String())

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
	metadataPath := filepath.Join(absPath, strings.TrimSuffix(filename, ".webm")+".json")
	data, _ := json.MarshalIndent(metadata, "", "  ")
	os.WriteFile(metadataPath, data, 0o644)
}

func loadMetadata(filename string) VideoMetadata {
	absPath, _ := filepath.Abs(config.MetadataDir)
	metadataPath := filepath.Join(absPath, strings.TrimSuffix(filename, ".webm")+".json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return VideoMetadata{}
	}
	var metadata VideoMetadata
	json.Unmarshal(data, &metadata)
	return metadata
}

func convertToWebm(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-c:v", "libvpx-vp9", "-c:a", "libopus", outputPath)
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".webm") {
			continue
		}
		thumbName := strings.TrimSuffix(entry.Name(), ".webm") + ".jpg"
		thumbPath := filepath.Join(config.ThumbnailsDir, thumbName)
		if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
			generateThumbnail(filepath.Join(config.VideoDir, entry.Name()), entry.Name())
		}
		info, _ := entry.Info()
		metadata := loadMetadata(entry.Name())
		title := metadata.Title
		if title == "" {
			title = strings.TrimSuffix(entry.Name(), ".webm")
		}
		videos = append(videos, Video{
			Name:      strings.TrimSuffix(entry.Name(), ".webm"),
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
	os.MkdirAll(absPath, 0o755)
	thumbName := strings.TrimSuffix(filename, ".webm") + ".jpg"
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".webm") {
			continue
		}
		thumbName := strings.TrimSuffix(entry.Name(), ".webm") + ".jpg"
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
	thumbName := strings.TrimSuffix(videoName, ".webm") + ".jpg"
	thumbPath := filepath.Join(config.ThumbnailsDir, thumbName)
	metadataName := strings.TrimSuffix(videoName, ".webm") + ".json"
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

		downloadedFile := tempPath + ".webm"
		if _, err := os.Stat(downloadedFile); os.IsNotExist(err) {
			files, _ := os.ReadDir(absPath)
			for _, f := range files {
				if strings.HasPrefix(f.Name(), job.Filename+".temp.") {
					downloadedFile = filepath.Join(absPath, f.Name())
					break
				}
			}
		}

		if strings.HasSuffix(downloadedFile, ".webm") {
			os.Rename(downloadedFile, videoPath)
		} else {
			if err := convertToWebm(downloadedFile, videoPath); err != nil {
				fmt.Printf("Failed to convert %s: %v\n", job.URL, err)
				os.Remove(downloadedFile)
				continue
			}
			os.Remove(downloadedFile)
		}

		downloadMutex.Lock()
		downloadStatus.Progress = 75
		downloadMutex.Unlock()

		saveMetadata(job.Filename, metadata)
		generateThumbnail(videoPath, job.Filename)

		downloadMutex.Lock()
		downloadStatus.Progress = 100
		downloadMutex.Unlock()

		fmt.Printf("Download complete: %s\n", job.Filename)
	}
}

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
