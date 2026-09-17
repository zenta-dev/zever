package main

import (
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

// TestDevSkipDir pins which directories stay unwatched: dot directories at
// any depth, plus dependency trees that churn on unrelated tool runs.
func TestDevSkipDir(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "proj")

	tests := []struct {
		name string
		path string
		base string
		want bool
	}{
		{name: "root itself", path: root, base: "proj", want: false},
		{name: "regular subdir", path: filepath.Join(root, "cmd"), base: "cmd", want: false},
		{name: "dot dir", path: filepath.Join(root, ".git"), base: ".git", want: true},
		{name: "nested dot dir", path: filepath.Join(root, "a", ".cache"), base: ".cache", want: true},
		{name: "node_modules", path: filepath.Join(root, "node_modules"), base: "node_modules", want: true},
		{name: "vendor", path: filepath.Join(root, "vendor"), base: "vendor", want: true},
		{name: "nested vendor", path: filepath.Join(root, "a", "vendor"), base: "vendor", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := devSkipDir(root, tt.path, tt.base); got != tt.want {
				t.Fatalf("devSkipDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestDevRelevantEvent pins the debounce trigger: only watched extensions,
// never chmod-only, never dotfiles.
func TestDevRelevantEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{name: "zen write", event: fsnotify.Event{Name: "schema/app.zen", Op: fsnotify.Write}, want: true},
		{name: "go write", event: fsnotify.Event{Name: "cmd/server/main.go", Op: fsnotify.Write}, want: true},
		{name: "go create", event: fsnotify.Event{Name: "cmd/server/new.go", Op: fsnotify.Create}, want: true},
		{name: "uppercase ext", event: fsnotify.Event{Name: "cmd/server/MAIN.GO", Op: fsnotify.Write}, want: true},
		{name: "chmod dropped", event: fsnotify.Event{Name: "schema/app.zen", Op: fsnotify.Chmod}, want: false},
		{name: "readme ignored", event: fsnotify.Event{Name: "README.md", Op: fsnotify.Write}, want: false},
		{name: "dotfile ignored", event: fsnotify.Event{Name: ".schema.swp", Op: fsnotify.Write}, want: false},
		{name: "hidden zen ignored", event: fsnotify.Event{Name: ".app.zen", Op: fsnotify.Write}, want: false},
		{name: "no extension", event: fsnotify.Event{Name: "Makefile", Op: fsnotify.Write}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := devRelevantEvent(tt.event); got != tt.want {
				t.Fatalf("devRelevantEvent(%+v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

// TestDevRelevantEventDirCreate pins the directory-create branch without a
// timer: a newly created directory matters even though it has no extension.
func TestDevRelevantEventDirCreate(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "newpkg")

	if err := mkdirZeverAll(sub); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	event := fsnotify.Event{Name: sub, Op: fsnotify.Create}
	if !devRelevantEvent(event) {
		t.Fatal("created directory should be a relevant event")
	}

	event = fsnotify.Event{Name: filepath.Join(dir, "missing"), Op: fsnotify.Create}
	if devRelevantEvent(event) {
		t.Fatal("created non-directory without watched extension should be irrelevant")
	}
}

// TestAddWatchTreeIsRecursive proves nested directories are watched, which
// fsnotify does not do on its own, and that churn-heavy trees stay out.
func TestAddWatchTreeIsRecursive(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")

	if err := mkdirZeverAll(nested); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := mkdirZeverAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := mkdirZeverAll(filepath.Join(dir, "vendor")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	watcher, err := newDevWatcher(ProjectConfig{SchemaDir: dir, ServerEntry: dir}.withDefaults())
	if err != nil {
		t.Fatalf("newDevWatcher: %v", err)
	}

	defer func() { _ = watcher.Close() }()

	watched := map[string]bool{}
	for _, path := range watcher.WatchList() {
		watched[path] = true
	}

	for _, want := range []string{dir, filepath.Join(dir, "a"), nested} {
		if !watched[want] {
			t.Errorf("directory %q is not watched (watch list: %v)", want, watcher.WatchList())
		}
	}

	for _, skip := range []string{filepath.Join(dir, ".git"), filepath.Join(dir, "vendor")} {
		if watched[skip] {
			t.Errorf("%q should be skipped", skip)
		}
	}
}

// TestNewDevWatcherToleratesMissingDirs proves watcher setup does not fail on
// a project whose conventional directories do not exist yet.
func TestNewDevWatcherToleratesMissingDirs(t *testing.T) {
	t.Chdir(t.TempDir())

	watcher, err := newDevWatcher(ProjectConfig{}.withDefaults())
	if err != nil {
		t.Fatalf("newDevWatcher: %v", err)
	}

	if err := watcher.Close(); err != nil {
		t.Fatalf("close watcher: %v", err)
	}
}

// TestAddWatchTreeRejectsFile pins the not-a-directory error branch.
func TestAddWatchTreeRejectsFile(t *testing.T) {
	dir := t.TempDir()
	file := writeZeverFixture(t, dir, "app.zen", "entity A { id: uuid @primary }\n")

	watcher, err := newDevWatcher(ProjectConfig{SchemaDir: file, ServerEntry: dir}.withDefaults())
	if err == nil {
		_ = watcher.Close()
		t.Fatal("expected error watching a file, got nil")
	}
}
