package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/internal/endpoint"
	"github.com/zenta-dev/zever/internal/httpclient"
)

type driver struct {
	endpoint  string
	apiKey    string
	maxOutput int64
	client    *http.Client
}

type renderRequest struct {
	Source []byte                `json:"source"`
	Format document.OutputFormat `json:"format"`
}

type renderResponse struct {
	Data []byte `json:"data"`
}

// Open creates a remote document renderer from the given Options.
func Open(o document.Options) (document.Document, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("remote: %w", err)
	}

	if o.Endpoint == "" {
		return nil, document.ErrMissingEndpoint
	}

	trimmed := strings.TrimRight(o.Endpoint, "/")
	normalized, err := endpoint.ValidateURL(trimmed,
		endpoint.WithAllowInsecure(true),
		endpoint.WithRejectQueryFragment(),
	)
	if err != nil {
		if errors.Is(err, endpoint.ErrQueryFragment) {
			return nil, fmt.Errorf("remote: endpoint %q must not contain a query string or fragment", o.Endpoint)
		}
		return nil, fmt.Errorf("remote: endpoint %q must be a valid http(s) URL", o.Endpoint)
	}

	timeout := o.Timeout
	if timeout == 0 {
		timeout = document.DefaultTimeout
	}

	maxOutput := o.MaxOutputBytes
	if maxOutput == 0 {
		maxOutput = document.DefaultMaxOutputBytes
	}

	return &driver{
		endpoint:  normalized,
		apiKey:    o.APIKey,
		maxOutput: maxOutput,
		client:    httpclient.NewClient(timeout),
	}, nil
}

func (d *driver) Render(ctx context.Context, source []byte, format document.OutputFormat) ([]byte, error) {
	switch format {
	case document.FormatPDF, document.FormatPNG, document.FormatJPG:
	default:
		var uerr error = &document.UnsupportedFormatError{Format: format}
		return nil, fmt.Errorf("remote: %w", uerr)
	}

	// renderRequest holds only []byte and string fields, so Marshal cannot fail.
	reqBody, _ := json.Marshal(renderRequest{Source: source, Format: format})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint+"/render", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("remote: request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote: render: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 513))
		if readErr != nil {
			return nil, fmt.Errorf("remote: read error body: %w", readErr)
		}

		if len(errBody) > 512 {
			errBody = errBody[:512]
		}

		return nil, fmt.Errorf("remote: render status %d: %q", resp.StatusCode, errBody)
	}

	body, err := httpclient.ReadLimited(resp.Body, d.maxOutput)
	if err != nil {
		if errors.Is(err, httpclient.ErrTooLarge) {
			var serr error = &document.SizeLimitError{Size: int(d.maxOutput) + 1, Limit: int(d.maxOutput)}
			return nil, fmt.Errorf("remote: %w", serr)
		}
		return nil, fmt.Errorf("remote: read: %w", err)
	}

	if len(body) == 0 {
		return nil, errors.New("remote: render: empty response data")
	}

	mt := ""

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		parsed, _, err := mime.ParseMediaType(ct)
		if err != nil {
			return nil, fmt.Errorf("remote: render: invalid content-type %q: %w", ct, err)
		}

		mt = parsed
	}

	if mt != "" && (strings.HasPrefix(mt, "image/") || mt == "application/pdf") {
		return body, nil
	}

	isJSON := mt == "application/json" || mt == "text/json"
	if !isJSON && sniffBinaryMagic(body) {
		return body, nil
	}

	if isJSON {
		var result renderResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("remote: decode: %w", err)
		}

		if len(result.Data) == 0 {
			return nil, errors.New("remote: render: empty response data")
		}

		return result.Data, nil
	}

	return nil, fmt.Errorf("remote: render: unexpected content-type %q", mt)
}

func sniffBinaryMagic(body []byte) bool {
	return bytes.HasPrefix(body, []byte("%PDF-")) ||
		bytes.HasPrefix(body, []byte("\x89PNG")) ||
		bytes.HasPrefix(body, []byte("\xff\xd8")) ||
		bytes.HasPrefix(body, []byte("GIF87a")) ||
		bytes.HasPrefix(body, []byte("GIF89a")) ||
		(len(body) >= 12 && bytes.HasPrefix(body, []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")))
}

// Close releases backend resources.
func (d *driver) Close() error {
	return nil
}
