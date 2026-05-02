package main

import (
	"time"
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
}
