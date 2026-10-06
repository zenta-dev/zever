package notification

import (
	"errors"
	"testing"
	"time"
)

func TestNotificationClone_preservesScalars(t *testing.T) {
	t.Parallel()

	n := Notification{
		Target:   "token",
		Channel:  ChannelPush,
		Title:    "t",
		Body:     "b",
		Priority: PriorityHigh,
		TTL:      time.Minute,
	}

	clone := n.Clone()

	if clone.Target != n.Target || clone.Channel != n.Channel || clone.Title != n.Title ||
		clone.Body != n.Body || clone.Priority != n.Priority || clone.TTL != n.TTL {
		t.Fatalf("Clone() = %+v, want %+v", clone, n)
	}
}

func TestNotificationClone_emptyNonNilData_staysNonNil(t *testing.T) {
	t.Parallel()

	n := Notification{Target: "token", Channel: ChannelPush, Body: "b", Data: map[string]string{}}
	clone := n.Clone()

	if clone.Data == nil {
		t.Fatal("Clone() Data = nil, want non-nil empty map")
	}
	if len(clone.Data) != 0 {
		t.Fatalf("Clone() Data len = %d, want 0", len(clone.Data))
	}
}

func TestOpen_invalidOptions_returnsInvalidOptions(t *testing.T) {
	t.Parallel()

	n, err := Open(freshAdapter(), Options{Timeout: -time.Second})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open invalid-opts err = %v, want ErrInvalidOptions", err)
	}
	if n != nil {
		t.Fatalf("Open invalid-opts notifier = %v, want nil", n)
	}
	var ve InvalidOptionsError
	if !errors.As(err, &ve) {
		t.Fatalf("err %T is not InvalidOptionsError", err)
	}
}
