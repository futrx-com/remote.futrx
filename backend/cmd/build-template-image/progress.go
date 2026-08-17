package main

import (
	"fmt"
	"log"

	serviceimage "github.com/futrx-com/remote.futrx.com/internal/service/container/image"
)

type logBuildProgressReporter struct {
	logger *log.Logger
}

func newLogBuildProgressReporter(logger *log.Logger) logBuildProgressReporter {
	return logBuildProgressReporter{logger: logger}
}

func (r logBuildProgressReporter) ReportImageBuildProgress(progress serviceimage.Progress) {
	label := fmt.Sprintf("[%d/%d] %s", progress.Stage, progress.StageCount, progress.Description)
	switch progress.State {
	case serviceimage.ProgressStarted:
		r.logger.Printf("%s...", label)
	case serviceimage.ProgressRunning:
		r.logger.Printf("%s still running (%s elapsed)", label, progress.Elapsed)
	case serviceimage.ProgressSucceeded:
		r.logger.Printf("%s finished in %s", label, progress.Elapsed)
	case serviceimage.ProgressFailed:
		r.logger.Printf("%s failed after %s", label, progress.Elapsed)
	}
}
