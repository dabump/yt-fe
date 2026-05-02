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
	Tags      []string
	TagsData  string
}

type VideoMetadata struct {
	Title        string   `json:"title"`
	VideoID      string   `json:"video_id"`
	URL          string   `json:"url"`
	Resolution   string   `json:"resolution,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	DownloadedAt string   `json:"downloaded_at"`
}

type PageData struct {
	Videos      []Video
	Tags        []string
	Error       string
	Success     string
	Downloading bool
	QueueCount  int
}

type DownloadJob struct {
	URL            string
	VideoID        string
	Filename       string
	FormatSelector string
	Resolution     string
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

type VideoFormatOption struct {
	Label          string `json:"label"`
	Height         int    `json:"height"`
	FormatSelector string `json:"format_selector"`
}

type VideoFormatsResponse struct {
	URL     string              `json:"url"`
	VideoID string              `json:"video_id"`
	Title   string              `json:"title"`
	Formats []VideoFormatOption `json:"formats"`
}

type TagsFile struct {
	Tags []string `json:"tags"`
}
