package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ytDLPFormat struct {
	Height int    `json:"height"`
	Vcodec string `json:"vcodec"`
}

func (app *App) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		app.handleForm(w, r)
		return
	}
	app.serveIndex(w, r, "", "")
}

func (app *App) handleForm(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	formatSelector := r.FormValue("format_selector")
	resolution := r.FormValue("resolution")
	if url == "" {
		app.serveIndex(w, r, "", "Please provide a YouTube URL")
		return
	}

	url = cleanYouTubeURL(url)

	videoID := extractVideoID(url)
	if videoID == "" {
		app.serveIndex(w, r, "", "Invalid YouTube URL. Please provide a valid YouTube video link (e.g., https://www.youtube.com/watch?v=VIDEO_ID)")
		return
	}

	if _, err := getVideoMetadata(url); err != nil {
		app.serveIndex(w, r, "", err.Error())
		return
	}
	if formatSelector == "" {
		formatSelector = "bestvideo+bestaudio/best"
	}
	if resolution == "" {
		resolution = "Best available"
	}

	filename := fmt.Sprintf("%s.webm", uuid.New().String())

	app.enqueueDownload(DownloadJob{
		URL:            url,
		VideoID:        videoID,
		Filename:       filename,
		FormatSelector: formatSelector,
		Resolution:     resolution,
	})

	app.serveIndex(w, r, "Video added to download queue!", "")
}

func (app *App) formatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := cleanYouTubeURL(r.FormValue("url"))
	if url == "" {
		http.Error(w, "Please provide a YouTube URL", http.StatusBadRequest)
		return
	}
	if extractVideoID(url) == "" {
		http.Error(w, "Invalid YouTube URL", http.StatusBadRequest)
		return
	}

	formats, err := getVideoFormats(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(formats); err != nil {
		log.Printf("Failed to encode formats response: %v", err)
	}
}

func getVideoFormats(url string) (VideoFormatsResponse, error) {
	cmd := exec.Command("yt-dlp", "--dump-json", "--no-download", "--no-warnings", url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "is not a valid URL") || strings.Contains(errMsg, "Unsupported URL") {
			return VideoFormatsResponse{}, fmt.Errorf("invalid YouTube URL: %s", url)
		}
		if strings.Contains(errMsg, "Video unavailable") || strings.Contains(errMsg, "is unavailable") {
			return VideoFormatsResponse{}, fmt.Errorf("this YouTube video is unavailable or has been removed")
		}
		return VideoFormatsResponse{}, fmt.Errorf("failed to get available video resolutions: %s", errMsg)
	}

	var data struct {
		Title     string        `json:"title"`
		DisplayID string        `json:"display_id"`
		ID        string        `json:"id"`
		Formats   []ytDLPFormat `json:"formats"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &data); err != nil {
		return VideoFormatsResponse{}, fmt.Errorf("failed to parse available video resolutions")
	}

	videoID := data.DisplayID
	if videoID == "" {
		videoID = data.ID
	}

	return VideoFormatsResponse{
		URL:     url,
		VideoID: videoID,
		Title:   data.Title,
		Formats: buildVideoFormatOptions(data.Formats),
	}, nil
}

func buildVideoFormatOptions(formats []ytDLPFormat) []VideoFormatOption {
	heights := map[int]bool{}
	for _, format := range formats {
		if format.Height <= 0 || format.Vcodec == "" || format.Vcodec == "none" {
			continue
		}
		heights[format.Height] = true
	}

	availableHeights := make([]int, 0, len(heights))
	for height := range heights {
		availableHeights = append(availableHeights, height)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(availableHeights)))

	options := []VideoFormatOption{{
		Label:          "Best available",
		Height:         0,
		FormatSelector: "bestvideo+bestaudio/best",
	}}
	for _, height := range availableHeights {
		options = append(options, VideoFormatOption{
			Label:          resolutionLabel(height),
			Height:         height,
			FormatSelector: fmt.Sprintf("bestvideo[height<=%d]+bestaudio/best[height<=%d]", height, height),
		})
	}

	return options
}

func resolutionLabel(height int) string {
	switch height {
	case 2160:
		return "4K / 2160p"
	case 1440:
		return "2K / 1440p"
	default:
		return fmt.Sprintf("%dp", height)
	}
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

func (app *App) saveMetadata(filename string, metadata VideoMetadata) error {
	absPath, err := filepath.Abs(app.config.MetadataDir)
	if err != nil {
		return err
	}
	metadataPath := filepath.Join(absPath, strings.TrimSuffix(filename, ".webm")+".json")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath, data, 0o644)
}

func (app *App) loadMetadata(filename string) VideoMetadata {
	absPath, err := filepath.Abs(app.config.MetadataDir)
	if err != nil {
		return VideoMetadata{}
	}
	metadataPath := filepath.Join(absPath, strings.TrimSuffix(filename, ".webm")+".json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return VideoMetadata{}
	}
	var metadata VideoMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return VideoMetadata{}
	}
	return metadata
}

func convertToWebm(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-c:v", "libvpx-vp9", "-c:a", "libopus", outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (app *App) serveIndex(w http.ResponseWriter, r *http.Request, success, errMsg string) {
	videos, err := app.getVideos()
	if err != nil {
		errMsg = fmt.Sprintf("Error reading videos: %v", err)
	}

	data := PageData{
		Videos:  videos,
		Error:   errMsg,
		Success: success,
	}

	if err := app.tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		return
	}
}

func (app *App) getVideos() ([]Video, error) {
	entries, err := os.ReadDir(app.config.VideoDir)
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
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		metadata := app.loadMetadata(entry.Name())
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

func (app *App) generateThumbnail(videoPath, filename string) {
	absPath, err := filepath.Abs(app.config.ThumbnailsDir)
	if err != nil {
		log.Printf("Failed to resolve thumbnail path: %v", err)
		return
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		log.Printf("Failed to create thumbnail directory: %v", err)
		return
	}
	thumbName := strings.TrimSuffix(filename, ".webm") + ".jpg"
	thumbPath := filepath.Join(absPath, thumbName)
	fmt.Printf("Generating thumbnail: ffmpeg -y -i %s -ss 00:00:01 -vframes 1 -q:v 2 %s\n", videoPath, thumbPath)
	cmd := exec.Command("ffmpeg", "-y", "-i", videoPath, "-ss", "00:00:01", "-vframes", "1", "-q:v", "2", thumbPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to generate thumbnail: %v", err)
	}
}

func (app *App) generateMissingThumbnails() {
	entries, err := os.ReadDir(app.config.VideoDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".webm") {
			continue
		}
		thumbName := strings.TrimSuffix(entry.Name(), ".webm") + ".jpg"
		thumbPath := filepath.Join(app.config.ThumbnailsDir, thumbName)
		if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
			videoPath := filepath.Join(app.config.VideoDir, entry.Name())
			fmt.Printf("Generating thumbnail for %s...\n", entry.Name())
			app.generateThumbnail(videoPath, entry.Name())
		}
	}
}

func (app *App) thumbnailHandler(w http.ResponseWriter, r *http.Request) {
	thumbName := filepath.Base(r.URL.Path)
	thumbPath := filepath.Join(app.config.ThumbnailsDir, thumbName)
	file, err := os.Open(thumbPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("Failed to serve thumbnail %s: %v", thumbName, err)
	}
}

func (app *App) downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	app.handleForm(w, r)
}

func (app *App) videoHandler(w http.ResponseWriter, r *http.Request) {
	videoName := filepath.Base(r.URL.Path)
	videoPath := filepath.Join(app.config.VideoDir, videoName)
	file, err := os.Open(videoPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "video/webm")
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("Failed to serve video %s: %v", videoName, err)
	}
}

func (app *App) deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	videoName := filepath.Base(r.URL.Path)
	videoPath := filepath.Join(app.config.VideoDir, videoName)
	thumbName := strings.TrimSuffix(videoName, ".webm") + ".jpg"
	thumbPath := filepath.Join(app.config.ThumbnailsDir, thumbName)
	metadataName := strings.TrimSuffix(videoName, ".webm") + ".json"
	metadataPath := filepath.Join(app.config.MetadataDir, metadataName)
	if err := removeIfExists(videoPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := removeIfExists(thumbPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := removeIfExists(metadataPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *App) processDownloadQueue() {
	for job := range app.jobs {
		app.startJob(job)
		fmt.Printf("Processing download: %s\n", job.URL)

		metadata, err := getVideoMetadata(job.URL)
		if err != nil {
			log.Printf("Failed to get metadata for %s: %v", job.URL, err)
			app.finishJob()
			continue
		}
		metadata.Resolution = job.Resolution

		absPath, err := filepath.Abs(app.config.VideoDir)
		if err != nil {
			log.Printf("Failed to resolve video path: %v", err)
			app.finishJob()
			continue
		}
		videoPath := filepath.Join(absPath, job.Filename)
		tempPath := videoPath + ".temp"

		outputTemplate := tempPath + ".%(ext)s"
		formatSelector := job.FormatSelector
		if formatSelector == "" {
			formatSelector = "bestvideo+bestaudio/best"
		}
		cmd := exec.Command("yt-dlp", "-f", formatSelector, "-o", outputTemplate, job.URL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			log.Printf("Failed to download %s: %v", job.URL, err)
			app.finishJob()
			continue
		}

		app.setDownloadProgress(50)

		downloadedFile := tempPath + ".webm"
		if _, err := os.Stat(downloadedFile); os.IsNotExist(err) {
			files, err := os.ReadDir(absPath)
			if err != nil {
				log.Printf("Failed to inspect download directory: %v", err)
				app.finishJob()
				continue
			}
			for _, f := range files {
				if strings.HasPrefix(f.Name(), job.Filename+".temp.") {
					downloadedFile = filepath.Join(absPath, f.Name())
					break
				}
			}
		}

		if strings.HasSuffix(downloadedFile, ".webm") {
			if err := os.Rename(downloadedFile, videoPath); err != nil {
				log.Printf("Failed to move downloaded file: %v", err)
				app.finishJob()
				continue
			}
		} else {
			if err := convertToWebm(downloadedFile, videoPath); err != nil {
				log.Printf("Failed to convert %s: %v", job.URL, err)
				if err := os.Remove(downloadedFile); err != nil && !os.IsNotExist(err) {
					log.Printf("Failed to remove temp download %s: %v", downloadedFile, err)
				}
				app.finishJob()
				continue
			}
			if err := os.Remove(downloadedFile); err != nil && !os.IsNotExist(err) {
				log.Printf("Failed to remove temp download %s: %v", downloadedFile, err)
			}
		}

		app.setDownloadProgress(75)

		if err := app.saveMetadata(job.Filename, metadata); err != nil {
			log.Printf("Failed to save metadata for %s: %v", job.Filename, err)
		}
		app.generateThumbnail(videoPath, job.Filename)

		app.setDownloadProgress(100)

		fmt.Printf("Download complete: %s\n", job.Filename)
		app.finishJob()
	}
}

func (app *App) apiStatusHandler(w http.ResponseWriter, r *http.Request) {
	snapshot := app.statusSnapshot()
	status := struct {
		Processing bool          `json:"processing"`
		Current    *DownloadJob  `json:"current"`
		Queue      []DownloadJob `json:"queue"`
		Progress   float64       `json:"progress"`
	}{
		Processing: snapshot.Processing,
		Current:    snapshot.Current,
		Queue:      snapshot.Queue,
		Progress:   snapshot.Progress,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("Failed to encode status response: %v", err)
	}
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}
