package run

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	api2convert "github.com/QaamGo/api2convert-go/v10"
	"github.com/fsnotify/fsnotify"
)

// WatchConfig configures the folder watcher.
type WatchConfig struct {
	Dir         string
	Target      string
	OutDir      string
	Recursive   bool
	Include     []string // glob patterns on basename; empty = all
	Exclude     []string
	Concurrency int // max simultaneous conversions (0 = NumCPU)
	Options     Options
}

// WatchHandler is called for each conversion outcome (or watcher error).
type WatchHandler func(res Result, err error)

// stabilizeDelay is how long a file's size must stay unchanged before we treat
// it as fully written and ready to convert.
const stabilizeDelay = 750 * time.Millisecond

// Watch monitors Dir for new/changed files and converts each into OutDir. It
// blocks until ctx is cancelled. Already-present files are not converted on
// startup; only files created/written after Watch starts are handled.
func Watch(ctx context.Context, c *api2convert.Client, cfg WatchConfig, onResult WatchHandler) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := addWatchDirs(w, cfg.Dir, cfg.Recursive); err != nil {
		return err
	}

	conc := cfg.Concurrency
	if conc <= 0 {
		conc = runtime.NumCPU()
	}
	sem := make(chan struct{}, conc)

	// Track outputs the watcher itself wrote, so we never reconvert them — this
	// is what stops an --on-conflict rename loop from converting a.webp →
	// a (1).webp → a (1) (1).webp … forever, burning quota. (The OutDir guard in
	// match() only helps when outputs land in a separate directory.)
	var wmu sync.Mutex
	written := map[string]struct{}{}
	markWritten := func(p string) {
		if abs, err := filepath.Abs(p); err == nil {
			wmu.Lock()
			written[abs] = struct{}{}
			wmu.Unlock()
		}
	}
	wasWritten := func(p string) bool {
		abs, err := filepath.Abs(p)
		if err != nil {
			return false
		}
		wmu.Lock()
		_, ok := written[abs]
		wmu.Unlock()
		return ok
	}

	// Debounce writes per path so a file that is still being copied is only
	// converted once it has been quiet for stabilizeDelay.
	var mu sync.Mutex
	timers := map[string]*time.Timer{}

	fire := func(path string) {
		if wasWritten(path) || !cfg.match(path) {
			return
		}
		// Bound simultaneous conversions the same way batch does, so dropping N
		// files never starts N parallel jobs.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-sem }()

		res, cerr := ConvertOne(ctx, c, path, cfg.Target, ensureDirArg(cfg.OutDir), cfg.Options, silentProgress())
		if cerr == nil && !res.Skipped && res.Path != "" {
			markWritten(res.Path)
		}
		onResult(res, cerr)
	}

	schedule := func(path string) {
		mu.Lock()
		defer mu.Unlock()
		if t, ok := timers[path]; ok {
			t.Stop()
		}
		timers[path] = time.AfterFunc(stabilizeDelay, func() {
			mu.Lock()
			delete(timers, path)
			mu.Unlock()
			fire(path)
		})
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			info, statErr := os.Stat(ev.Name)
			if statErr != nil {
				continue
			}
			if info.IsDir() {
				if cfg.Recursive && ev.Op&fsnotify.Create != 0 {
					_ = w.Add(ev.Name) // watch newly created subdirectories
				}
				continue
			}
			schedule(ev.Name)
		case werr, ok := <-w.Errors:
			if !ok {
				return nil
			}
			onResult(Result{}, werr)
		}
	}
}

func addWatchDirs(w *fsnotify.Watcher, root string, recursive bool) error {
	if !recursive {
		return w.Add(root)
	}
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return w.Add(p)
		}
		return nil
	})
}

func (cfg WatchConfig) match(path string) bool {
	base := filepath.Base(path)
	for _, ex := range cfg.Exclude {
		if ok, _ := filepath.Match(ex, base); ok {
			return false
		}
	}
	// Don't reconvert our own outputs sitting in the same tree.
	if cfg.OutDir != "" {
		if abs, err := filepath.Abs(path); err == nil {
			if outAbs, err2 := filepath.Abs(cfg.OutDir); err2 == nil && strings.HasPrefix(abs, outAbs+string(os.PathSeparator)) {
				return false
			}
		}
	}
	if len(cfg.Include) == 0 {
		return true
	}
	for _, in := range cfg.Include {
		if ok, _ := filepath.Match(in, base); ok {
			return true
		}
	}
	return false
}

func ensureDirArg(dir string) string {
	if dir == "" {
		return ""
	}
	return strings.TrimRight(dir, `/\`) + string(os.PathSeparator)
}
