package storage

import "context"

// FireAllow emits an allowed policy decision to the decision hook.
func FireAllow(ctx context.Context, perm Perm, bucket string, subject Subject, reason Reason, version string) {
	FireDecision(ctx, Decision{
		Allow:   true,
		Perm:    perm,
		Bucket:  bucket,
		Subject: subject,
		Reason:  reason,
		Version: version,
	})
}

// FireDeny emits a denied policy decision to the decision hook.
func FireDeny(ctx context.Context, perm Perm, bucket string, subject Subject, reason Reason, version string) {
	FireDecision(ctx, Decision{
		Allow:   false,
		Perm:    perm,
		Bucket:  bucket,
		Subject: subject,
		Reason:  reason,
		Version: version,
	})
}
