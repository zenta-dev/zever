package notification

import (
	"errors"
	"testing"
)

func TestAdapter_FCM_valueAndName(t *testing.T) {
	t.Parallel()
	if int(FCM) != 2 {
		t.Fatalf("FCM = %d, want 2 (Log=0, Twilio=1 reserved, FCM=2)", int(FCM))
	}
	if got := FCM.String(); got != "fcm" {
		t.Errorf("FCM.String() = %q, want %q", got, "fcm")
	}
}

func TestAdapter_Parse_fcm(t *testing.T) {
	t.Parallel()
	got, err := ParseAdapter("fcm")
	if err != nil {
		t.Fatalf("ParseAdapter(fcm) err = %v", err)
	}
	if got != FCM {
		t.Fatalf("ParseAdapter(fcm) = %v, want FCM", got)
	}
	if got.String() != "fcm" {
		t.Errorf("roundtrip String() = %q, want %q", got.String(), "fcm")
	}
	var iae *InvalidAdapterError
	if _, err := ParseAdapter("FCM"); err == nil || !errors.As(err, &iae) {
		t.Errorf("ParseAdapter(FCM) err = %v, want *InvalidAdapterError", err)
	}
}
