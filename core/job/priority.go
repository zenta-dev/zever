package job

// Priority orders job execution urgency, with higher values dequeued first.
type Priority uint8

const (
	// PriorityLow marks background jobs dequeued after higher priorities.
	PriorityLow = iota
	// PriorityMedium marks standard jobs dequeued before low priority.
	PriorityMedium
	// PriorityHigh marks urgent jobs dequeued before medium and low priorities.
	PriorityHigh
)

// String returns the human-readable name of the Priority.
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	default:
		return "unknown"
	}
}
