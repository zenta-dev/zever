package memory

import (
	"container/list"
	"testing"
	"time"
)

func TestMemorySweepBranches(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	// Empty map: no-op.
	a := &memoryAdapter{items: make(map[string]item), index: make(map[string]*list.Element), l: list.New()}
	a.sweep()
	if len(a.items) != 0 {
		t.Fatalf("empty sweep left %d items", len(a.items))
	}

	// Unexpired entry is kept.
	a.items["fresh"] = item{value: []byte("v"), expiresAt: future}
	a.sweep()
	if _, ok := a.items["fresh"]; !ok {
		t.Fatal("sweep removed unexpired entry")
	}

	// Expired entry without index is removed.
	a.items["gone"] = item{value: []byte("v"), expiresAt: past}
	a.sweep()
	if _, ok := a.items["gone"]; ok {
		t.Fatal("sweep kept expired entry")
	}
	if _, ok := a.items["fresh"]; !ok {
		t.Fatal("sweep removed unexpired entry alongside expired one")
	}

	// Expired entry with index + list is removed from all three.
	el := a.l.PushFront("tracked")
	a.index["tracked"] = el
	a.items["tracked"] = item{value: []byte("v"), expiresAt: past}
	a.sweep()
	if _, ok := a.items["tracked"]; ok {
		t.Fatal("sweep kept indexed expired entry")
	}
	if _, ok := a.index["tracked"]; ok {
		t.Fatal("sweep kept index entry")
	}
	for e := a.l.Front(); e != nil; e = e.Next() {
		if e == el {
			t.Fatal("sweep kept list element")
		}
	}

	// Expired entry with index hit but nil list skips removal safely.
	other := list.New()
	b := &memoryAdapter{
		items: map[string]item{"k": {value: []byte("v"), expiresAt: past}},
		index: map[string]*list.Element{"k": other.PushFront("k")},
		l:     nil,
	}
	b.sweep()
	if _, ok := b.items["k"]; ok {
		t.Fatal("sweep kept expired entry with nil list")
	}
	if _, ok := b.index["k"]; ok {
		t.Fatal("sweep kept index entry with nil list")
	}
}
