package outbox

// PublisherSetter attaches the relay's destination after the store is opened.
//
// core/outbox.Options carries only the informational transport selector string
// (Options.Publisher), never a Publisher value: the transport itself (a
// resolved core/queue.Queue or core/eventbus.EventBus) belongs to application
// wiring and therefore does not exist yet at Open time. container.OutboxRelay
// resolves the transport first and hands the store the derived Publisher
// through this interface, so the relay becomes startable from a container
// rather than from hand-rolled wiring.
//
// It is a separate optional interface, never a method on Store, so existing
// Store implementations keep compiling (the same reasoning as Admin).
//
// Implementations must be safe for concurrent use, must ignore a nil
// Publisher, and must make Start fail rather than silently no-op when no
// publisher was ever attached: a relay that claims messages and drops them is
// worse than a boot failure.
type PublisherSetter interface {
	// SetPublisher attaches p as the relay's destination.
	SetPublisher(p Publisher)
}
