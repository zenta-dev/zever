package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// devWatchedExts are the file kinds a change to which is worth a rebuild: the
// schema DSL itself, and the Go source of the entrypoint being run. Everything
// else under those trees (editor swap files, generated artefacts, READMEs) is
// ignored, which keeps a single save from cascading into repeated restarts.
var devWatchedExts = map[string]bool{
	".zen": true,
	".go":  true,
}

// newDevWatcher sets up the fsnotify watcher for a dev session: the schema tree
// and the server entrypoint tree, both recursive.
//
// Recursion is the caller's job — fsnotify watches a single directory, not its
// subdirectories — so both roots are walked with filepath.WalkDir and every
// directory found is added individually. The entrypoint tree is watched
// recursively too, because an entrypoint package commonly grows sibling
// subpackages the developer expects to trigger a restart.
//
// A missing root is not an error: a project may legitimately have no schema
// directory yet, and refusing to start dev over that would be unhelpful.
func newDevWatcher(project ProjectConfig) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("%s: create watcher: %w", devTag, err)
	}

	for _, root := range []string{project.SchemaDir, project.ServerEntry} {
		if err := addWatchTree(watcher, root); err != nil {
			_ = watcher.Close()

			return nil, err
		}
	}

	return watcher, nil
}

// addWatchTree adds root and every directory beneath it to the watcher.
func addWatchTree(watcher *fsnotify.Watcher, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("%s: stat %q: %w", devTag, root, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s: %q is not a directory", devTag, root)
	}

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() {
			return nil
		}

		if devSkipDir(root, path, entry.Name()) {
			return filepath.SkipDir
		}

		if addErr := watcher.Add(path); addErr != nil {
			return fmt.Errorf("watch %q: %w", path, addErr)
		}

		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("%s: %w", devTag, walkErr)
	}

	return nil
}

// devSkipDir reports whether a directory should be left unwatched. Dot
// directories (.git above all, whose index churns constantly) would otherwise
// fire events on every unrelated tool run; dependency trees (vendor,
// node_modules) churn on every unrelated install.
func devSkipDir(root, path, name string) bool {
	if path == root {
		return false
	}

	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor"
}

// devRelevantEvent reports whether an event should reset the debounce timer.
// Chmod-only events are dropped: they carry no content change, and some
// editors and filesystems emit a stream of them. Dotfiles (editor swap and
// lock files) are dropped whatever their extension.
func devRelevantEvent(event fsnotify.Event) bool {
	if event.Op == fsnotify.Chmod {
		return false
	}

	if strings.HasPrefix(filepath.Base(event.Name), ".") {
		return false
	}

	// A directory create still matters — it may be a new package about to
	// receive watched files — and has no extension to match on.
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			return true
		}
	}

	return devWatchedExts[strings.ToLower(filepath.Ext(event.Name))]
}
