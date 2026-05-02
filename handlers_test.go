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
