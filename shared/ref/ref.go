package ref

// DefaultName is the instance name Default assigns.
const DefaultName = "default"

// Ref names one wired service instance: the battery interface T built from
// Options O, addressed by Service (the config service key, e.g. "db") and
// Name (the instance name, "default" unless renamed).
//
// The type parameters never appear in the fields, so a Ref carries no value
// at runtime: it is only a compile-time-checked key. Refs with equal
// Service and Name denote the same instance. Ref is comparable and usable
// as a map key.
type Ref[T, O any] struct {
	Service string
	Name    string
}

// Named returns the Ref for the named instance of service.
func Named[T, O any](service, name string) Ref[T, O] {
	return Ref[T, O]{Service: service, Name: name}
}

// Default returns the Ref for the default instance of service, named
// "default".
func Default[T, O any](service string) Ref[T, O] {
	return Ref[T, O]{Service: service, Name: DefaultName}
}

// WithName returns the Ref for the same service under another instance
// name. The receiver is unchanged.
func (r Ref[T, O]) WithName(name string) Ref[T, O] {
	r.Name = name

	return r
}

// String renders the Ref as the diagnostics label "service" for the
// default instance or "service/name" otherwise. It carries no option or
// secret content, only the address.
func (r Ref[T, O]) String() string {
	if r.Name == "" || r.Name == DefaultName {
		return r.Service
	}

	return r.Service + "/" + r.Name
}
