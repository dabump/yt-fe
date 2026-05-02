package main

import "testing"

func TestExtractVideoID(t *testing.T) {
	tests := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":              "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=example": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                             "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=42":                        "dQw4w9WgXcQ",
		"https://example.com/watch?v=dQw4w9WgXcQ":                  "dQw4w9WgXcQ",
		"not a youtube url":                                        "",
		"https://youtu.be/short":                                   "",
	}

	for rawURL, expected := range tests {
		if actual := extractVideoID(rawURL); actual != expected {
			t.Fatalf("extractVideoID(%q) = %q, want %q", rawURL, actual, expected)
		}
	}
}

func TestCleanYouTubeURL(t *testing.T) {
	tests := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=example": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=42":                        "https://youtu.be/dQw4w9WgXcQ",
		"not a youtube url":                                        "not a youtube url",
	}

	for rawURL, expected := range tests {
		if actual := cleanYouTubeURL(rawURL); actual != expected {
			t.Fatalf("cleanYouTubeURL(%q) = %q, want %q", rawURL, actual, expected)
		}
	}
}

func TestBuildVideoFormatOptions(t *testing.T) {
	options := buildVideoFormatOptions([]ytDLPFormat{
		{Height: 720, Vcodec: "vp9"},
		{Height: 2160, Vcodec: "vp9"},
		{Height: 1440, Vcodec: "av01"},
		{Height: 720, Vcodec: "avc1"},
		{Height: 0, Vcodec: "vp9"},
		{Height: 1080, Vcodec: "none"},
	})

	expectedLabels := []string{"Best available", "4K / 2160p", "2K / 1440p", "720p"}
	if len(options) != len(expectedLabels) {
		t.Fatalf("len(options) = %d, want %d", len(options), len(expectedLabels))
	}

	for i, expected := range expectedLabels {
		if options[i].Label != expected {
			t.Fatalf("options[%d].Label = %q, want %q", i, options[i].Label, expected)
		}
	}

	if options[1].FormatSelector != "bestvideo[height<=2160]+bestaudio/best[height<=2160]" {
		t.Fatalf("4K selector = %q", options[1].FormatSelector)
	}
}

func TestResolutionLabel(t *testing.T) {
	tests := map[int]string{
		2160: "4K / 2160p",
		1440: "2K / 1440p",
		1080: "1080p",
	}

	for height, expected := range tests {
		if actual := resolutionLabel(height); actual != expected {
			t.Fatalf("resolutionLabel(%d) = %q, want %q", height, actual, expected)
		}
	}
}
