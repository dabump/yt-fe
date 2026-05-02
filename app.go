package main

import (
	"html/template"
	"os"
	"sync"
)

const downloadQueueSize = 100

type App struct {
	config Config
	tmpl   *template.Template

	jobs          chan DownloadJob
	statusMutex   sync.Mutex
	tagMutex      sync.Mutex
	pendingJobs   []DownloadJob
	downloadState DownloadStatus
}

func NewApp(cfg Config) (*App, error) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		return nil, err
	}

	return &App{
		config:        cfg,
		tmpl:          tmpl,
		jobs:          make(chan DownloadJob, downloadQueueSize),
		downloadState: DownloadStatus{Queue: []DownloadJob{}},
	}, nil
}

func (app *App) ensureDirectories() error {
	for _, dir := range []string{app.config.VideoDir, app.config.ThumbnailsDir, app.config.MetadataDir, tagsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return app.ensureTagsFile()
}

func (app *App) enqueueDownload(job DownloadJob) {
	app.statusMutex.Lock()
	app.pendingJobs = append(app.pendingJobs, job)
	app.downloadState.Queue = cloneJobs(app.pendingJobs)
	app.statusMutex.Unlock()

	app.jobs <- job
}

func (app *App) startJob(job DownloadJob) {
	app.statusMutex.Lock()
	if len(app.pendingJobs) > 0 {
		app.pendingJobs = app.pendingJobs[1:]
	}
	app.downloadState.Queue = cloneJobs(app.pendingJobs)
	app.downloadState.Processing = true
	app.downloadState.Current = &job
	app.downloadState.Progress = 0
	app.statusMutex.Unlock()
}

func (app *App) setDownloadProgress(progress float64) {
	app.statusMutex.Lock()
	app.downloadState.Progress = progress
	app.statusMutex.Unlock()
}

func (app *App) finishJob() {
	app.statusMutex.Lock()
	app.downloadState.Processing = false
	app.downloadState.Current = nil
	app.downloadState.Progress = 0
	app.downloadState.Queue = cloneJobs(app.pendingJobs)
	app.statusMutex.Unlock()
}

func (app *App) statusSnapshot() DownloadStatus {
	app.statusMutex.Lock()
	defer app.statusMutex.Unlock()

	return DownloadStatus{
		Current:    app.downloadState.Current,
		Queue:      cloneJobs(app.downloadState.Queue),
		Processing: app.downloadState.Processing,
		Progress:   app.downloadState.Progress,
	}
}

func cloneJobs(jobs []DownloadJob) []DownloadJob {
	if len(jobs) == 0 {
		return []DownloadJob{}
	}
	return append([]DownloadJob(nil), jobs...)
}
