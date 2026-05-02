package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	tagsDir      = "tags"
	tagsFileName = "tags.json"
)

func tagsFilePath() string {
	return filepath.Join(tagsDir, tagsFileName)
}

func normalizeTag(tag string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(tag, ",", " ")), " ")
}

func sameTag(a, b string) bool {
	return strings.EqualFold(normalizeTag(a), normalizeTag(b))
}

func (app *App) ensureTagsFile() error {
	app.tagMutex.Lock()
	defer app.tagMutex.Unlock()

	if _, err := os.Stat(tagsFilePath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	return app.writeTagsLocked([]string{})
}

func (app *App) loadTags() ([]string, error) {
	app.tagMutex.Lock()
	defer app.tagMutex.Unlock()
	return app.loadTagsLocked()
}

func (app *App) loadTagsLocked() ([]string, error) {
	data, err := os.ReadFile(tagsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var tagsFile TagsFile
	if len(strings.TrimSpace(string(data))) == 0 {
		return []string{}, nil
	}
	if err := json.Unmarshal(data, &tagsFile); err != nil {
		return nil, err
	}

	return normalizeTags(tagsFile.Tags), nil
}

func (app *App) writeTagsLocked(tags []string) error {
	data, err := json.MarshalIndent(TagsFile{Tags: normalizeTags(tags)}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tagsFilePath(), data, 0o644)
}

func normalizeTags(tags []string) []string {
	seen := map[string]string{}
	for _, tag := range tags {
		normalized := normalizeTag(tag)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := seen[key]; !exists {
			seen[key] = normalized
		}
	}

	result := make([]string, 0, len(seen))
	for _, tag := range seen {
		result = append(result, tag)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

func (app *App) createTag(tag string) ([]string, error) {
	app.tagMutex.Lock()
	defer app.tagMutex.Unlock()

	normalized := normalizeTag(tag)
	if normalized == "" {
		return nil, fmt.Errorf("tag name is required")
	}

	tags, err := app.loadTagsLocked()
	if err != nil {
		return nil, err
	}
	for _, existing := range tags {
		if sameTag(existing, normalized) {
			return nil, fmt.Errorf("tag already exists")
		}
	}

	tags = append(tags, normalized)
	tags = normalizeTags(tags)
	return tags, app.writeTagsLocked(tags)
}

func (app *App) renameTag(oldTag, newTag string) ([]string, error) {
	app.tagMutex.Lock()
	defer app.tagMutex.Unlock()

	oldTag = normalizeTag(oldTag)
	newTag = normalizeTag(newTag)
	if oldTag == "" || newTag == "" {
		return nil, fmt.Errorf("tag name is required")
	}

	tags, err := app.loadTagsLocked()
	if err != nil {
		return nil, err
	}
	found := false
	for i, tag := range tags {
		if sameTag(tag, oldTag) {
			tags[i] = newTag
			found = true
			continue
		}
		if sameTag(tag, newTag) {
			return nil, fmt.Errorf("tag already exists")
		}
	}
	if !found {
		return nil, fmt.Errorf("tag not found")
	}

	tags = normalizeTags(tags)
	if err := app.writeTagsLocked(tags); err != nil {
		return nil, err
	}
	return tags, app.replaceTagInAllMetadataLocked(oldTag, newTag)
}

func (app *App) deleteTag(tag string) ([]string, error) {
	app.tagMutex.Lock()
	defer app.tagMutex.Unlock()

	tag = normalizeTag(tag)
	if tag == "" {
		return nil, fmt.Errorf("tag name is required")
	}

	tags, err := app.loadTagsLocked()
	if err != nil {
		return nil, err
	}
	updated := make([]string, 0, len(tags))
	found := false
	for _, existing := range tags {
		if sameTag(existing, tag) {
			found = true
			continue
		}
		updated = append(updated, existing)
	}
	if !found {
		return nil, fmt.Errorf("tag not found")
	}

	if err := app.writeTagsLocked(updated); err != nil {
		return nil, err
	}
	return updated, app.removeTagFromAllMetadataLocked(tag)
}

func (app *App) updateVideoTags(filename string, tags []string) ([]string, error) {
	app.tagMutex.Lock()
	defer app.tagMutex.Unlock()

	knownTags, err := app.loadTagsLocked()
	if err != nil {
		return nil, err
	}
	allowed := map[string]string{}
	for _, tag := range knownTags {
		allowed[strings.ToLower(tag)] = tag
	}

	selected := make([]string, 0, len(tags))
	for _, tag := range normalizeTags(tags) {
		known, exists := allowed[strings.ToLower(tag)]
		if !exists {
			return nil, fmt.Errorf("unknown tag: %s", tag)
		}
		selected = append(selected, known)
	}

	metadata, err := app.loadMetadataByFilename(filename)
	if err != nil {
		return nil, err
	}
	metadata.Tags = selected
	if err := app.saveMetadataByFilename(filename, metadata); err != nil {
		return nil, err
	}
	return selected, nil
}

func (app *App) removeTagFromAllMetadataLocked(tag string) error {
	return app.rewriteAllMetadataTagsLocked(func(tags []string) []string {
		updated := make([]string, 0, len(tags))
		for _, existing := range tags {
			if !sameTag(existing, tag) {
				updated = append(updated, existing)
			}
		}
		return updated
	})
}

func (app *App) replaceTagInAllMetadataLocked(oldTag, newTag string) error {
	return app.rewriteAllMetadataTagsLocked(func(tags []string) []string {
		updated := make([]string, 0, len(tags))
		for _, existing := range tags {
			if sameTag(existing, oldTag) {
				updated = append(updated, newTag)
				continue
			}
			updated = append(updated, existing)
		}
		return normalizeTags(updated)
	})
}

func (app *App) rewriteAllMetadataTagsLocked(rewrite func([]string) []string) error {
	entries, err := os.ReadDir(app.config.MetadataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filename := strings.TrimSuffix(entry.Name(), ".json") + ".webm"
		metadata, err := app.loadMetadataByFilename(filename)
		if err != nil {
			return err
		}
		metadata.Tags = rewrite(metadata.Tags)
		if err := app.saveMetadataByFilename(filename, metadata); err != nil {
			return err
		}
	}
	return nil
}
