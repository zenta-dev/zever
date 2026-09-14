package notification

import (
	"testing"
)

func TestOptions_FCM_zeroValue(t *testing.T) {
	t.Parallel()
	var o Options
	if o.FCM.ProjectID != "" || o.FCM.ServiceAccount != "" {
		t.Fatalf("zero Options.FCM = %+v, want empty", o.FCM)
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("zero Options with FCM field Validate() = %v, want nil", err)
	}
}

func TestOptions_FCM_fieldsPassthrough(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.FCM.ProjectID = "p"
	o.FCM.ServiceAccount = "svc.json"
	if o.FCM.ProjectID != "p" || o.FCM.ServiceAccount != "svc.json" {
		t.Fatalf("Options.FCM = %+v, want passthrough", o.FCM)
	}
}
