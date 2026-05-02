package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

func defaultConfig() Config {
	return Config{
		VideoDir:      "video",
		ThumbnailsDir: "thumbnails",
		MetadataDir:   "metadata",
	}
}

func loadConfig() Config {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return defaultConfig()
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return defaultConfig()
	}
	return cfg
}
