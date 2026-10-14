package main

import (
	"strings"
	"testing"
)

// TestResolveBuildAdaptersEmptyMeansFull pins the empty contract: no
// --adapters flag builds the full CLI (every registered adapter).
func TestResolveBuildAdaptersEmptyMeansFull(t *testing.T) {
	t.Parallel()

	sel, err := resolveBuildAdapters("")
	if err != nil {
		t.Fatalf("resolveBuildAdapters(%q) error = %v", "", err)
	}

	if sel != nil {
		t.Fatalf("resolveBuildAdapters(%q) = %v, want nil (full set)", "", sel)
	}
}

// TestResolveBuildAdaptersBareBatteryExpands pins that a bare battery name
// selects every known adapter for that battery.
func TestResolveBuildAdaptersBareBatteryExpands(t *testing.T) {
	t.Parallel()

	sel, err := resolveBuildAdapters("cache")
	if err != nil {
		t.Fatalf("resolveBuildAdapters cache error = %v", err)
	}

	want := adaptersForBattery("cache")
	if len(sel) != len(want) {
		t.Fatalf("resolveBuildAdapters cache = %v, want %d selections %v", sel, len(want), want)
	}

	got := make([]string, 0, len(sel))
	for _, b := range sel {
		if b.Battery != "cache" {
			t.Errorf("sel entry = %+v, want battery cache", b)
		}

		got = append(got, b.Adapter)
	}

	wantSet := map[string]bool{}
	for _, a := range want {
		wantSet[a] = true
	}

	for _, a := range got {
		if !wantSet[a] {
			t.Errorf("adapter %q not in adaptersForBattery(cache) = %v", a, want)
		}
	}
}

// TestResolveBuildAdaptersPinNarrows pins battery/adapter pins to one entry.
func TestResolveBuildAdaptersPinNarrows(t *testing.T) {
	t.Parallel()

	sel, err := resolveBuildAdapters("cache/redis, log/slog")
	if err != nil {
		t.Fatalf("resolveBuildAdapters pin error = %v", err)
	}

	if len(sel) != 2 {
		t.Fatalf("resolveBuildAdapters pin = %v, want 2 selections", sel)
	}

	if sel[0].Battery != "cache" || sel[0].Adapter != "redis" {
		t.Errorf("sel[0] = %+v, want cache/redis", sel[0])
	}

	if sel[1].Battery != "log" || sel[1].Adapter != "slog" {
		t.Errorf("sel[1] = %+v, want log/slog", sel[1])
	}
}

// TestResolveBuildAdaptersUnknownBatteryFailsClosed pins the unknown error.
func TestResolveBuildAdaptersUnknownBatteryFailsClosed(t *testing.T) {
	t.Parallel()

	_, err := resolveBuildAdapters("nope")
	if err == nil {
		t.Fatal("resolveBuildAdapters nope = nil error, want unknown adapter error")
	}

	if !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("error %q missing battery name", err.Error())
	}
}

// TestResolveBuildAdaptersUnknownAdapterFailsClosed pins unknown adapters.
func TestResolveBuildAdaptersUnknownAdapterFailsClosed(t *testing.T) {
	t.Parallel()

	_, err := resolveBuildAdapters("cache/nope")
	if err == nil {
		t.Fatal("resolveBuildAdapters cache/nope = nil error, want unknown adapter error")
	}

	if !strings.Contains(err.Error(), `"cache/nope"`) {
		t.Errorf("error %q missing selection", err.Error())
	}
}

// TestResolveBuildAdaptersDedupesAndSorts pins normalization.
func TestResolveBuildAdaptersDedupesAndSorts(t *testing.T) {
	t.Parallel()

	sel, err := resolveBuildAdapters("queue/memory, cache, cache/memory")
	if err != nil {
		t.Fatalf("resolveBuildAdapters error = %v", err)
	}

	for i := 1; i < len(sel); i++ {
		prev := sel[i-1].Battery + "/" + sel[i-1].Adapter
		cur := sel[i].Battery + "/" + sel[i].Adapter
		if prev >= cur {
			t.Fatalf("selections not sorted/deduped: %v", sel)
		}
	}
}

// TestResolveBuildAdaptersLegacyAlias pins consolidated-dir aliases.
func TestResolveBuildAdaptersLegacyAlias(t *testing.T) {
	t.Parallel()

	sel, err := resolveBuildAdapters("search/postgres")
	if err != nil {
		t.Fatalf("resolveBuildAdapters search/postgres error = %v", err)
	}

	if len(sel) != 1 || sel[0].Battery != "search" {
		t.Fatalf("resolveBuildAdapters search/postgres = %v, want one search selection", sel)
	}
}
