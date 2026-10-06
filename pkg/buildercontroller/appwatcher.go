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

	// Close waits this long for the watcher's goroutines. On Windows a queued fsnotify Add
	// may never get its reply once the watcher is closed, so an unbounded wait could hang
	// DeleteController and Shutdown; leaking one stuck goroutine is the lesser harm.
	WatcherCloseWaitTimeout = 2 * time.Second
)

const (
	defaultRootAddRetryDelay = 25 * time.Millisecond
	rootAddAttempts          = 3
)

var (
	// Spacing of the Add retries when the root's listing loses a race with a vanishing file.
	rootAddRetryDelay    = defaultRootAddRetryDelay
	watchDebounce        = 300 * time.Millisecond
	rootPollInterval     = 1 * time.Second
	rootUnavailableAfter = 10 * time.Second
	// A directory cap bounds inotify watches (one per directory). On macOS kqueue opens a
	// file descriptor per watched file, so this cap does not bound descriptor use there.
	maxWatchedDirs = 1000
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
	addFn       func(dir string) error
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
		addFn:       fsw.Add,
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
	w.shutdown(true)
}

// A goroutine owned by the watcher's WaitGroup cannot wait for the group, so the
// watcher stops itself with wait=false; a later Close() still waits for the goroutines.
func (w *AppWatcher) shutdown(wait bool) {
	first := w.markClosed()
	if first {
		close(w.closeCh)
		w.fsw.Close()
	}
	if wait {
		w.waitBounded()
	}
	if first {
		releaseWatcherSlot()
	}
}

func (w *AppWatcher) waitBounded() {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(WatcherCloseWaitTimeout):
		log.Printf("app watcher %s: goroutines still running %v after Close; not waiting for them\n", w.appDir, WatcherCloseWaitTimeout)
	}
}

// "Unavailable" has to mean stopped: a watcher that reports it but keeps its slot and
// inotify descriptor would hold them until the builder window closes.
func (w *AppWatcher) abort(reason string) {
	if w.isClosed() {
		return
	}
	w.onStatus(WatchStatus_Unavailable, reason)
	w.shutdown(false)
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
			// A directory that vanished mid-scan (an editor's temp directory, renamed
			// away) is not a reason to stop watching.
			if path == root && !errors.Is(walkErr, fs.ErrNotExist) {
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

// The directory is reserved in the map before the Add, and the lock is never held across
// the Add: on Windows fsnotify's Add and Remove wait for a reply from the backend's reader,
// which can be blocked sending to Events, so a lock held here would also block Close.
func (w *AppWatcher) addWatch(dir string) error {
	addFn, err := w.reserveWatch(dir)
	if err != nil || addFn == nil {
		return err
	}
	err = addFn(dir)
	if err != nil && dir == w.appDir && errors.Is(err, fs.ErrNotExist) {
		err = w.retryRootAdd(addFn, dir, err)
	}
	if err != nil {
		w.releaseWatch(dir)
		if errors.Is(err, fs.ErrNotExist) && w.canIgnoreMissing(dir) {
			return nil
		}
		return fmt.Errorf("cannot watch %s: %w", dir, err)
	}
	return nil
}

// On kqueue Add lists the directory, and an entry that vanishes between the listing and its
// Lstat fails the whole Add with ErrNotExist. While the folder itself still exists that is a
// lost race with whoever deleted the file, and a retry normally gets through.
func (w *AppWatcher) retryRootAdd(addFn func(string) error, dir string, err error) error {
	for attempt := 1; attempt < rootAddAttempts && errors.Is(err, fs.ErrNotExist); attempt++ {
		if _, statErr := os.Lstat(dir); statErr != nil {
			return err
		}
		select {
		case <-w.closeCh:
			return err
		case <-time.After(rootAddRetryDelay):
		}
		err = addFn(dir)
	}
	return err
}

// A subdirectory that vanished mid-scan is never fatal. The root is forgiven only while it is
// still there: a missing root is the caller's cue to poll for it, not a watcher to keep.
func (w *AppWatcher) canIgnoreMissing(dir string) bool {
	if dir != w.appDir {
		return true
	}
	info, err := os.Lstat(dir)
	return err == nil && info.IsDir()
}

func (w *AppWatcher) reserveWatch(dir string) (func(string) error, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed || w.watchedDirs[dir] {
		return nil, nil
	}
	if len(w.watchedDirs) >= maxWatchedDirs {
		return nil, fmt.Errorf("more than %d folders to watch in %s", maxWatchedDirs, w.appDir)
	}
	w.watchedDirs[dir] = true
	return w.addFn, nil
}

func (w *AppWatcher) releaseWatch(dir string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	delete(w.watchedDirs, dir)
}

// Only the map is updated: fsnotify drops the watch of a deleted directory itself, and
// calling Remove from the goroutine that reads Events can deadlock on Windows.
func (w *AppWatcher) forgetWatches(dir string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	prefix := dir + string(filepath.Separator)
	for watched := range w.watchedDirs {
		if watched == dir || strings.HasPrefix(watched, prefix) {
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
			w.scanNewDirAsync(name)
			return
		}
	}
	if IsRelevantAppPath(rel) {
		w.scheduleChange()
	}
}

// Adding watches is kept off the goroutine that reads Events (see addWatch). The scan
// is followed by a change, because files created in the directory before its watch
// existed produced no events.
func (w *AppWatcher) scanNewDirAsync(dir string) {
	if w.isClosed() {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			panichandler.PanicHandler("AppWatcher.scanNewDir", recover())
		}()
		if err := w.addDirTree(dir); err != nil {
			w.abort(err.Error())
			return
		}
		w.scheduleChange()
	}()
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
				if errors.Is(err, fs.ErrNotExist) {
					// The folder vanished again before the scan finished. Keep polling, unless
					// its removal event already started another poller.
					if w.markRootMissing() {
						w.forgetWatches(w.appDir)
						continue
					}
					return
				}
				w.abort(err.Error())
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
