// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/panichandler"
	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/fsnotify/fsnotify"
)

const (
	WatchStatus_Active      = "active"
	WatchStatus_Unavailable = "unavailable"
	MaxAppWatchers          = 8
)

var (
	watchDebounce        = 300 * time.Millisecond
	rootPollInterval     = 1 * time.Second
	rootUnavailableAfter = 10 * time.Second
	maxWatchedDirs       = 1000
)

var (
	watcherSlotLock sync.Mutex
	watcherSlots    int
)

type AppWatcher struct {
	lock        sync.Mutex
	appDir      string
	fsw         *fsnotify.Watcher
	watchedDirs map[string]bool
	onChange    func()
	onStatus    func(status string, reason string)
	debounce    *time.Timer
	closed      bool
	rootMissing bool
	closeCh     chan struct{}
	wg          sync.WaitGroup
}

// The global cap bounds inotify usage across builder windows; each watcher holds one
// inotify instance plus one watch per directory.
func acquireWatcherSlot() bool {
	watcherSlotLock.Lock()
	defer watcherSlotLock.Unlock()
	if watcherSlots >= MaxAppWatchers {
		return false
	}
	watcherSlots++
	return true
}

func releaseWatcherSlot() {
	watcherSlotLock.Lock()
	defer watcherSlotLock.Unlock()
	watcherSlots--
}

func activeWatcherCount() int {
	watcherSlotLock.Lock()
	defer watcherSlotLock.Unlock()
	return watcherSlots
}

func MakeAppWatcher(appDir string, onChange func(), onStatus func(status string, reason string)) (*AppWatcher, error) {
	if !acquireWatcherSlot() {
		return nil, fmt.Errorf("too many app folders are being watched (limit %d)", MaxAppWatchers)
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		releaseWatcherSlot()
		return nil, fmt.Errorf("cannot start a file watcher: %w", err)
	}
	w := &AppWatcher{
		appDir:      filepath.Clean(appDir),
		fsw:         fsw,
		watchedDirs: make(map[string]bool),
		onChange:    onChange,
		onStatus:    onStatus,
		closeCh:     make(chan struct{}),
	}
	if err := w.addTree(); err != nil {
		fsw.Close()
		releaseWatcherSlot()
		return nil, err
	}
	w.wg.Add(1)
	go w.run()
	return w, nil
}

func (w *AppWatcher) Close() {
	if !w.markClosed() {
		return
	}
	close(w.closeCh)
	w.fsw.Close()
	w.wg.Wait()
	releaseWatcherSlot()
}

func (w *AppWatcher) markClosed() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed {
		return false
	}
	w.closed = true
	if w.debounce != nil {
		w.debounce.Stop()
	}
	return true
}

func (w *AppWatcher) isClosed() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.closed
}

func (w *AppWatcher) addTree() error {
	if err := remotetermappstore.CheckNoSymlinks(w.appDir); err != nil {
		return err
	}
	info, err := os.Lstat(w.appDir)
	if err != nil {
		return fmt.Errorf("app folder %s is not available: %w", w.appDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("app folder %s is not a directory", w.appDir)
	}
	if err := w.addWatch(w.appDir); err != nil {
		return err
	}
	staticDir := filepath.Join(w.appDir, "static")
	if info, err := os.Lstat(staticDir); err == nil && info.IsDir() {
		return w.addDirTree(staticDir)
	}
	return nil
}

// WalkDir never follows symlinked directories, so a link planted under static/
// cannot pull an outside tree into the watch set.
func (w *AppWatcher) addDirTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isSkippedWatchDir(d.Name()) {
			return fs.SkipDir
		}
		return w.addWatch(path)
	})
}

func isSkippedWatchDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

func isWatchableNewDir(rel string) bool {
	parts := strings.Split(rel, "/")
	if parts[0] != "static" {
		return false
	}
	for _, part := range parts {
		if isSkippedWatchDir(part) {
			return false
		}
	}
	return true
}

func (w *AppWatcher) addWatch(dir string) error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed || w.watchedDirs[dir] {
		return nil
	}
	if len(w.watchedDirs) >= maxWatchedDirs {
		return fmt.Errorf("more than %d folders to watch in %s", maxWatchedDirs, w.appDir)
	}
	if err := w.fsw.Add(dir); err != nil {
		return fmt.Errorf("cannot watch %s: %w", dir, err)
	}
	w.watchedDirs[dir] = true
	return nil
}

func (w *AppWatcher) forgetWatches(dir string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	prefix := dir + string(filepath.Separator)
	for watched := range w.watchedDirs {
		if watched == dir || strings.HasPrefix(watched, prefix) {
			w.fsw.Remove(watched)
			delete(w.watchedDirs, watched)
		}
	}
}

func (w *AppWatcher) run() {
	defer w.wg.Done()
	defer func() {
		panichandler.PanicHandler("AppWatcher.run", recover())
	}()
	for {
		select {
		case <-w.closeCh:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("app watcher %s: %v\n", w.appDir, err)
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.scheduleChange()
			}
		}
	}
}

func (w *AppWatcher) handleEvent(event fsnotify.Event) {
	if event.Op == fsnotify.Chmod {
		return
	}
	name := filepath.Clean(event.Name)
	if name == w.appDir {
		if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
			w.handleRootLost()
		}
		return
	}
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		w.forgetWatches(name)
	}
	rel, err := filepath.Rel(w.appDir, name)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if event.Op&fsnotify.Create != 0 && isWatchableNewDir(rel) {
		if info, err := os.Lstat(name); err == nil && info.IsDir() {
			if err := w.addDirTree(name); err != nil {
				w.onStatus(WatchStatus_Unavailable, err.Error())
			}
			w.scheduleChange()
			return
		}
	}
	if IsRelevantAppPath(rel) {
		w.scheduleChange()
	}
}

func (w *AppWatcher) handleRootLost() {
	if !w.markRootMissing() {
		return
	}
	w.forgetWatches(w.appDir)
	w.wg.Add(1)
	go w.pollRoot()
}

func (w *AppWatcher) markRootMissing() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed || w.rootMissing {
		return false
	}
	w.rootMissing = true
	return true
}

func (w *AppWatcher) setRootPresent() {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.rootMissing = false
}

// Deleting or renaming the app folder drops every inotify watch; polling is the only
// way to notice it coming back (git checkout, an agent recreating the folder).
func (w *AppWatcher) pollRoot() {
	defer w.wg.Done()
	defer func() {
		panichandler.PanicHandler("AppWatcher.pollRoot", recover())
	}()
	start := time.Now()
	reported := false
	ticker := time.NewTicker(rootPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.closeCh:
			return
		case <-ticker.C:
		}
		if info, err := os.Lstat(w.appDir); err == nil && info.IsDir() {
			w.setRootPresent()
			if err := w.addTree(); err != nil {
				w.onStatus(WatchStatus_Unavailable, err.Error())
				return
			}
			w.onStatus(WatchStatus_Active, "")
			w.scheduleChange()
			return
		}
		if !reported && time.Since(start) >= rootUnavailableAfter {
			reported = true
			w.onStatus(WatchStatus_Unavailable, "the app folder is missing")
		}
	}
}

func (w *AppWatcher) scheduleChange() {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed {
		return
	}
	if w.debounce != nil {
		w.debounce.Stop()
	}
	w.debounce = time.AfterFunc(watchDebounce, w.fireChange)
}

func (w *AppWatcher) fireChange() {
	defer func() {
		panichandler.PanicHandler("AppWatcher.fireChange", recover())
	}()
	if w.isClosed() {
		return
	}
	w.onChange()
}
