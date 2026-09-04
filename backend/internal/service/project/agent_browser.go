package project

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// projectLister is the slice of Repository the idle reaper needs.
type projectLister interface {
	List(ctx context.Context) ([]Meta, error)
}

// agentBrowsers owns the Agent Browser lifecycle across projects: the
// starting/ready/error state machine, the activity heartbeats that feed idle
// reaping, and the reaper goroutine itself. A nil ContainerBrowser disables the
// capability, and every operation degrades to the no-browser path. Safe for
// concurrent use.
type agentBrowsers struct {
	browser  ContainerBrowser
	projects projectLister

	stateMu sync.Mutex
	info    map[ID]AgentBrowserInfo
	// startID counts start attempts per project so a superseded background
	// start cannot publish its outcome over a newer one.
	startID map[ID]int64

	activityMu sync.Mutex
	seen       map[ID]time.Time
	reaperOn   bool
}

func newAgentBrowsers(browser ContainerBrowser, projects projectLister) *agentBrowsers {
	return &agentBrowsers{
		browser:  browser,
		projects: projects,
		info:     make(map[ID]AgentBrowserInfo),
		startID:  make(map[ID]int64),
		seen:     make(map[ID]time.Time),
	}
}

// start provisions the Agent Browser for a project whose container is already
// running, ensuring the stack in the background. Idempotent while a start is
// already in flight.
func (b *agentBrowsers) start(ctx context.Context, id ID, m Meta) (AgentBrowserInfo, error) {
	if b.browser == nil || m.ContainerName == "" {
		return AgentBrowserInfo{}, errors.New("project has no container to run the browser in")
	}
	b.touch(id)
	if info, ok := b.state(id); ok && info.Status == AgentBrowserStatusStarting {
		return info, nil
	}
	current, err := b.browser.Status(ctx, m.ContainerName)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	if current.Status == AgentBrowserStatusReady {
		current.Slug = m.Slug
		current.Port = b.browser.Port()
		current.LastActivity = b.lastActivity(id)
		b.setState(id, current)
		return current, nil
	}

	info, startID := b.beginStart(id, m)
	go b.ensureStarted(id, startID, m)
	return info, nil
}

// status returns split core/view browser state and records a heartbeat so an
// open pane keeps the idle reaper from tearing down the stack.
func (b *agentBrowsers) status(ctx context.Context, id ID, m Meta) (AgentBrowserInfo, error) {
	b.touch(id)
	info := b.infoFor(m, AgentBrowserStatusStopped, "")
	if b.browser == nil || m.ContainerName == "" {
		info.LastActivity = b.lastActivity(id)
		return info, nil
	}
	stored, hasStored := b.state(id)
	containerInfo, err := b.browser.Status(ctx, m.ContainerName)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	containerInfo.Slug = m.Slug
	containerInfo.Port = b.browser.Port()
	containerInfo.LastActivity = b.lastActivity(id)
	if containerInfo.Core == "ready" {
		info = containerInfo
		if info.Status == AgentBrowserStatusCoreReady && info.View == "ready" {
			info.Status = AgentBrowserStatusReady
		}
		b.setState(id, info)
		return info, nil
	}
	if hasStored {
		switch stored.Status {
		case AgentBrowserStatusStarting, AgentBrowserStatusError:
			stored.LastActivity = b.lastActivity(id)
			return stored, nil
		case AgentBrowserStatusReady:
			b.clearState(id)
		}
	}
	info.Core = containerInfo.Core
	info.View = containerInfo.View
	info.ViewerCount = containerInfo.ViewerCount
	info.UptimeSec = containerInfo.UptimeSec
	info.LastActivity = b.lastActivity(id)
	return info, nil
}

// stop tears down the Agent Browser stack in the project's container, leaving
// the container running and the persistent browser profile on disk so logins
// survive.
func (b *agentBrowsers) stop(ctx context.Context, id ID, m Meta) error {
	if b.browser == nil || m.ContainerName == "" {
		b.clearState(id)
		b.forgetActivity(id)
		return nil
	}
	b.clearState(id)
	if err := b.browser.Stop(ctx, m.ContainerName); err != nil {
		return err
	}
	b.forgetActivity(id)
	return nil
}

// stopView tears down only the human noVNC layer.
func (b *agentBrowsers) stopView(ctx context.Context, m Meta) error {
	if b.browser == nil || m.ContainerName == "" {
		return nil
	}
	return b.browser.StopView(ctx, m.ContainerName)
}

// stopBeforeUpgrade stops Chromium gracefully so it can remove its Singleton*
// locks before the disposable container is deleted — browser profiles are
// durable host mounts. A missing or legacy browser stack must not block the
// workspace upgrade; the replacement container provisions it on demand.
func (b *agentBrowsers) stopBeforeUpgrade(ctx context.Context, id ID, containerName string) {
	if b.browser == nil {
		return
	}
	if err := b.browser.Stop(ctx, containerName); err != nil {
		log.Printf("projects: stop agent browser in %s before upgrade: %v", containerName, err)
	}
	b.clearState(id)
	b.forgetActivity(id)
}

func (b *agentBrowsers) ensureStarted(id ID, startID int64, m Meta) {
	if err := b.browser.Ensure(context.Background(), m.ContainerName); err != nil {
		log.Printf("projects: start agent browser for %s: %v", id, err)
		b.finishStart(id, startID, b.infoFor(m, AgentBrowserStatusError, err.Error()))
		return
	}
	b.finishStart(id, startID, b.infoFor(m, AgentBrowserStatusReady, ""))
}

func (b *agentBrowsers) infoFor(m Meta, status AgentBrowserStatus, errMsg string) AgentBrowserInfo {
	info := AgentBrowserInfo{
		Status: status,
		Slug:   m.Slug,
		Error:  errMsg,
	}
	if b.browser != nil {
		info.Port = b.browser.Port()
	}
	return info
}

func (b *agentBrowsers) state(id ID) (AgentBrowserInfo, bool) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	info, ok := b.info[id]
	return info, ok
}

func (b *agentBrowsers) setState(id ID, info AgentBrowserInfo) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	b.info[id] = info
}

func (b *agentBrowsers) beginStart(id ID, m Meta) (AgentBrowserInfo, int64) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	b.startID[id]++
	startID := b.startID[id]
	info := b.infoFor(m, AgentBrowserStatusStarting, "")
	b.info[id] = info
	return info, startID
}

func (b *agentBrowsers) finishStart(id ID, startID int64, info AgentBrowserInfo) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.startID[id] != startID {
		return
	}
	b.info[id] = info
}

// clearState drops the project's published state and invalidates any start
// still in flight, so its outcome is discarded rather than resurrecting the
// entry.
func (b *agentBrowsers) clearState(id ID) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	b.startID[id]++
	delete(b.info, id)
}

// touch records a browser-use heartbeat for idle reaping.
func (b *agentBrowsers) touch(id ID) {
	if !ValidID(id) {
		return
	}
	b.activityMu.Lock()
	defer b.activityMu.Unlock()
	b.seen[id] = time.Now()
}

func (b *agentBrowsers) lastActivity(id ID) int64 {
	b.activityMu.Lock()
	defer b.activityMu.Unlock()
	t := b.seen[id]
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func (b *agentBrowsers) forgetActivity(id ID) {
	b.activityMu.Lock()
	defer b.activityMu.Unlock()
	delete(b.seen, id)
}

// startReaper stops browser stacks that have had no agent or pane activity for
// ttl. It is safe to call multiple times; only the first call starts a ticker.
func (b *agentBrowsers) startReaper(ctx context.Context, ttl time.Duration) {
	if ttl <= 0 || b.browser == nil {
		return
	}
	b.activityMu.Lock()
	if b.reaperOn {
		b.activityMu.Unlock()
		return
	}
	b.reaperOn = true
	b.activityMu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.reapIdle(ttl)
			}
		}
	}()
}

func (b *agentBrowsers) reapIdle(ttl time.Duration) {
	metas, err := b.projects.List(context.Background())
	if err != nil {
		log.Printf("projects: browser reaper list: %v", err)
		return
	}
	now := time.Now()
	for _, m := range metas {
		if m.ContainerName == "" {
			continue
		}
		b.activityMu.Lock()
		last, ok := b.seen[m.ID]
		if !ok || last.IsZero() {
			b.seen[m.ID] = now
			b.activityMu.Unlock()
			continue
		}
		idle := now.Sub(last)
		b.activityMu.Unlock()
		if idle < ttl {
			continue
		}
		statusCtx, statusCancel := context.WithTimeout(context.Background(), 10*time.Second)
		info, statusErr := b.browser.Status(statusCtx, m.ContainerName)
		statusCancel()
		if statusErr != nil {
			continue
		}
		if info.Core != "ready" {
			b.clearState(m.ID)
			b.forgetActivity(m.ID)
			continue
		}
		if info.ViewerCount > 0 {
			b.touch(m.ID)
			continue
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := b.browser.Stop(stopCtx, m.ContainerName)
		stopCancel()
		if err != nil {
			log.Printf("projects: browser reap %s/%s after %s: %v", m.ID, m.ContainerName, idle.Round(time.Second), err)
		} else {
			b.clearState(m.ID)
			b.forgetActivity(m.ID)
			log.Printf("projects: browser reap %s/%s after %s", m.ID, m.ContainerName, idle.Round(time.Second))
		}
	}
}
