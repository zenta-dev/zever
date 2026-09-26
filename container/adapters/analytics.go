package adapters

import (
	"github.com/zenta-dev/zever/analytics"
	analyticsposthog "github.com/zenta-dev/zever/analytics/posthog"
)

// RegisterAnalytics registers the SaaS-backed analytics adapter the core
// container no longer imports: analytics/posthog. The log adapter stays
// wired by the container. Registration only fills factory maps; it
// performs no I/O. Duplicate registrations are ignored, so calling
// RegisterAnalytics more than once (or alongside RegisterAll) is safe.
func RegisterAnalytics() {
	_ = analytics.Register(analytics.PostHog, analyticsposthog.New)
}
