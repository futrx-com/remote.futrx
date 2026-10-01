package serverinfo

import (
	"context"
	"time"
)

type Collector interface {
	Collect(ctx context.Context, now time.Time) Snapshot
}

type Service struct {
	warningPercent float64
	collector      Collector
	appVersion     string
	dataPath       string
	workspacePath  string
	startedAt      time.Time
}

func New(collector Collector, appVersion, dataPath, workspacePath string) *Service {
	return &Service{
		warningPercent: 80,
		collector:      collector,
		appVersion:     appVersion,
		dataPath:       dataPath,
		workspacePath:  workspacePath,
		startedAt:      time.Now(),
	}
}

func (s *Service) Collect(ctx context.Context) Info {
	now := time.Now()
	snapshot := s.collector.Collect(ctx, now)
	for i, mount := range snapshot.Storage.Mounts {
		snapshot.Storage.Mounts[i].Warning = mount.UsagePercent >= s.warningPercent || (mount.InodePercent != nil && *mount.InodePercent >= s.warningPercent)
	}
	snapshot.Host.ServiceUptimeSec = int64(now.Sub(s.startedAt).Seconds())
	snapshot.Host.AppVersion = s.appVersion
	snapshot.Host.DataPath = s.dataPath
	snapshot.Host.WorkspacePath = s.workspacePath
	return Info{
		CollectedAt: now.Unix(),
		Host:        snapshot.Host,
		CPU:         snapshot.CPU,
		Memory:      snapshot.Memory,
		Storage:     snapshot.Storage,
		Network:     snapshot.Network,
		Process:     snapshot.Process,
	}
}

func (s *Service) WithStorageWarningThreshold(percent float64) *Service {
	if percent > 0 && percent <= 100 {
		s.warningPercent = percent
	}
	return s
}
