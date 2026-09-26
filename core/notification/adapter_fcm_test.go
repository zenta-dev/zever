package notification

import (
	"testing"
)

func TestAdapter_FCM_valueAndName(t *testing.T) {
	t.Parallel()
	if FCM != "fcm" {
		t.Fatalf("FCM = %q, want %q", string(FCM), "fcm")
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
	_ = iae
	gotUpper, err := ParseAdapter("FCM")
	if err != nil {
		t.Errorf("ParseAdapter(FCM) err = %v, want nil (open adapter)", err)
	} else if gotUpper != Adapter("FCM") {
		t.Errorf("ParseAdapter(FCM) = %v, want %v", gotUpper, Adapter("FCM"))
	}
}
