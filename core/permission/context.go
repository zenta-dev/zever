package permission

import "context"

type subjectKey struct{}

// cloneSubject deep-copies the slice/map members so context storage never
// aliases caller memory: mutating the caller's or the retrieved Subject
// must not affect the other.
func cloneSubject(s Subject) Subject {
	cp := s
	cp.Roles = append([]string(nil), s.Roles...)
	if s.Attributes != nil {
		attrs := make(map[string]string, len(s.Attributes))
		for k, v := range s.Attributes {
			attrs[k] = v
		}
		cp.Attributes = attrs
	}
	return cp
}

// WithSubject returns a context carrying the given subject.
// Roles and Attributes are copied; later mutations by the caller do not
// affect the stored subject.
func WithSubject(ctx context.Context, s Subject) context.Context {
	return context.WithValue(ctx, subjectKey{}, cloneSubject(s))
}

// SubjectFrom returns the subject stored in ctx, if any.
// The returned Roles and Attributes are copies; mutating them never
// affects the stored subject or later lookups.
func SubjectFrom(ctx context.Context) (Subject, bool) {
	s, ok := ctx.Value(subjectKey{}).(Subject)
	if !ok {
		return Subject{}, false
	}
	return cloneSubject(s), true
}
