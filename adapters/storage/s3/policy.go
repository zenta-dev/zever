package s3

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awstransport "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/storage"
)

const (
	policyVersion     = "2012-10-17"
	zeverSidPrefix    = "zever-"
	policyResourceFmt = "arn:aws:s3:::%s/*"
)

// PolicySynchronizer reconciles configured policy with live bucket policy.
// PolicySynchronizer is implemented by s3Adapter with behavior controlled by policy_sync and sync_fail modes.
type PolicySynchronizer interface {
	SyncPolicy(ctx context.Context, bucket string) error
}

var _ PolicySynchronizer = (*s3Adapter)(nil)

type bucketPolicyStatement struct {
	Sid       string `json:"Sid"`
	Effect    string `json:"Effect"`
	Principal struct {
		AWS string `json:"AWS"`
	} `json:"Principal"`
	Action   string   `json:"Action"`
	Resource []string `json:"Resource"`
}

type bucketPolicyDocument struct {
	Version   string                  `json:"Version"`
	Statement []bucketPolicyStatement `json:"Statement"`
}

type rawPolicyDocument struct {
	Version   string           `json:"Version"`
	Statement []jsontext.Value `json:"Statement"`
}

// bucketPolicyDoc renders the managed-policy document for p. It cannot fail:
// the document is a struct of strings, so json.Marshal always succeeds.
func bucketPolicyDoc(p storage.Policy, bucket string) string {
	type permerAction struct {
		perm   storage.Perm
		action string
	}

	order := []permerAction{
		{storage.PermRead, "s3:GetObject"},
		{storage.PermWrite, "s3:PutObject"},
		{storage.PermDelete, "s3:DeleteObject"},
		{storage.PermUpdate, "s3:PutObject"},
	}

	doc := bucketPolicyDocument{Version: policyVersion, Statement: []bucketPolicyStatement{}}

	for _, pa := range order {
		if !p.Public(pa.perm) {
			continue
		}

		var st bucketPolicyStatement

		st.Sid = zeverSidPrefix + string(pa.perm)
		st.Effect = "Allow"
		st.Principal.AWS = "*"
		st.Action = pa.action
		st.Resource = []string{fmt.Sprintf(policyResourceFmt, bucket)}
		doc.Statement = append(doc.Statement, st)
	}

	b, _ := json.Marshal(doc, jsontext.WithIndent("  "))

	return string(b)
}

// SyncPolicy is a no-op unless policy_sync is auto, otherwise it runs the get, merge, and put bucket policy flow while firing drift and sync-error decisions.
// SyncPolicy returns the sync error when sync_fail is require, else it warns and returns nil.
func (a *s3Adapter) SyncPolicy(ctx context.Context, bucket string) error {
	if a.policySync != "auto" {
		return nil
	}

	current, err := a.getBucketPolicy(ctx, bucket)
	if err != nil {
		return a.policySyncError(ctx, bucket, fmt.Errorf("s3: get bucket policy: %w", err))
	}

	merged, drift, err := a.mergePolicy(bucket, current)
	if err != nil {
		return a.policySyncError(ctx, bucket, err)
	}

	if drift {
		storage.FireDecision(ctx, storage.Decision{
			Allow:   false,
			Bucket:  bucket,
			Reason:  storage.ReasonPolicyDrift,
			Version: a.PolicyFor(bucket).Version,
		})
	}

	// mergePolicy already parsed current into the same document shape, and
	// merged is json.Marshal output, so neither canonicalization can fail.
	canonCurrent, _ := canonicalPolicy(current)
	canonMerged, _ := canonicalPolicy(merged)

	if canonMerged != canonCurrent {
		if err := a.putBucketPolicy(ctx, bucket, merged); err != nil {
			return a.policySyncError(ctx, bucket, fmt.Errorf("s3: put bucket policy: %w", err))
		}
	}

	return nil
}

func (a *s3Adapter) policySyncError(ctx context.Context, bucket string, err error) error {
	if a.syncFail == "require" {
		return err
	}

	storage.FireDecision(ctx, storage.Decision{
		Allow:   false,
		Bucket:  bucket,
		Reason:  storage.ReasonPolicySyncError,
		Version: a.PolicyFor(bucket).Version,
	})

	return nil
}

func (a *s3Adapter) mergePolicy(bucket, current string) (string, bool, error) {
	genDoc := bucketPolicyDoc(a.PolicyFor(bucket), bucket)

	// genDoc is json.Marshal output, so it always parses.
	var gen rawPolicyDocument
	_ = json.Unmarshal([]byte(genDoc), &gen)

	existing := rawPolicyDocument{Version: policyVersion, Statement: []jsontext.Value{}}
	if strings.TrimSpace(current) != "" {
		if err := json.Unmarshal([]byte(current), &existing); err != nil {
			return "", false, fmt.Errorf("s3: parse existing bucket policy: %w", err)
		}

		if existing.Version == "" {
			existing.Version = policyVersion
		}
	}

	if existing.Statement == nil {
		existing.Statement = []jsontext.Value{}
	}

	var kept, ours []jsontext.Value

	for _, st := range existing.Statement {
		var sidHolder struct {
			Sid string `json:"Sid"`
		}
		if err := json.Unmarshal(st, &sidHolder); err != nil {
			kept = append(kept, st)
			continue
		}
		if strings.HasPrefix(sidHolder.Sid, zeverSidPrefix) {
			ours = append(ours, st)
		} else {
			kept = append(kept, st)
		}
	}

	canonOurs := canonicalStatements(ours)
	canonGen := canonicalStatements(gen.Statement)

	drift := len(ours) > 0 && canonOurs != canonGen

	merged := rawPolicyDocument{Version: existing.Version, Statement: append(kept, gen.Statement...)}

	// Every statement above survived a strict unmarshal (gen is Marshal
	// output), so this re-marshal cannot fail.
	b, _ := json.Marshal(merged, jsontext.WithIndent("  "))

	return string(b), drift, nil
}

func canonicalPolicy(s string) (string, error) {
	var doc rawPolicyDocument
	if strings.TrimSpace(s) == "" {
		doc = rawPolicyDocument{Version: policyVersion, Statement: []jsontext.Value{}}
		return marshalRawPolicy(doc)
	}

	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return "", fmt.Errorf("s3: parse bucket policy: %w", err)
	}

	if doc.Version == "" {
		doc.Version = policyVersion
	}

	if doc.Statement == nil {
		doc.Statement = []jsontext.Value{}
	}

	return marshalRawPolicy(doc)
}

func marshalRawPolicy(doc rawPolicyDocument) (string, error) {
	b, err := json.Marshal(doc, jsontext.WithIndent("  "))
	if err != nil {
		return "", fmt.Errorf("s3: marshal bucket policy: %w", err)
	}

	return string(b), nil
}

// canonicalStatements renders stmts in canonical sorted form. Callers only
// pass statements that already survived a strict unmarshal, so neither the
// re-unmarshal nor the deterministic re-marshal can fail.
func canonicalStatements(stmts []jsontext.Value) string {
	parts := make([]string, 0, len(stmts))

	for _, st := range stmts {
		var m map[string]jsontext.Value
		_ = json.Unmarshal(st, &m)

		b, _ := json.Marshal(m, json.Deterministic(true))

		parts = append(parts, string(b))
	}

	sort.Strings(parts)

	return strings.Join(parts, "\n")
}

func (a *s3Adapter) getBucketPolicy(ctx context.Context, bucket string) (string, error) {
	out, err := a.Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		if out.Policy == nil {
			return "", nil
		}

		return *out.Policy, nil
	}

	var respErr *awstransport.ResponseError
	if errors.As(err, &respErr) && respErr.Response.StatusCode == http.StatusNotFound {
		return "", nil
	}

	return "", err
}

func (a *s3Adapter) putBucketPolicy(ctx context.Context, bucket, doc string) error {
	_, err := a.Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(doc),
	})

	return err
}
