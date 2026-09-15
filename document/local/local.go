package local

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/zenta-dev/zever/document"
)

const maxSourceBytes = document.DefaultMaxSourceBytes

type driver struct {
	tmpDir    string
	tmpDirSet bool
	timeout   time.Duration
	quality   int64
	//nolint:containedctx // chromedp requires a persistent parent allocator context; closed by Close.
	allocCtx    context.Context
	allocCancel context.CancelFunc
	closeOnce   sync.Once
}

// Open creates a local document renderer from the given Options.
func Open(o document.Options) (document.Document, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}

	d := &driver{
		tmpDir:  o.TmpDir,
		timeout: o.Timeout,
		quality: int64(document.ClampQuality(o.Quality)),
	}

	if d.tmpDir == "" {
		d.tmpDir = "/tmp"
	} else {
		d.tmpDirSet = true
	}

	if d.timeout == 0 {
		d.timeout = document.DefaultTimeout
	}

	var allocOpts []chromedp.ExecAllocatorOption

	allocOpts = append(allocOpts, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts, chromedp.Headless)

	// Some CI/container runners have unprivileged user namespaces disabled
	// (e.g. AppArmor-restricted Ubuntu images), which makes Chrome's own
	// sandbox refuse to start at all ("No usable sandbox!"). Opt-in only,
	// via an explicit env var never set in a normal deployment, so this
	// never silently weakens the sandbox outside a controlled CI runner.
	if os.Getenv("ZEVER_CHROMEDP_NO_SANDBOX") != "" {
		allocOpts = append(allocOpts, chromedp.NoSandbox)
	}

	// Long-lived allocator, closed via driver.Close.
	d.allocCtx, d.allocCancel = chromedp.NewExecAllocator(context.Background(), allocOpts...)

	return d, nil
}

func (d *driver) Render(ctx context.Context, source []byte, format document.OutputFormat) ([]byte, error) {
	switch format {
	case document.FormatPDF, document.FormatPNG, document.FormatJPG:
	default:
		formatErr := error(&document.UnsupportedFormatError{Format: format})
		return nil, fmt.Errorf("local: %w", formatErr)
	}

	if len(source) > maxSourceBytes {
		sizeErr := error(&document.SizeLimitError{Size: len(source), Limit: maxSourceBytes})
		return nil, fmt.Errorf("local: %w", sizeErr)
	}

	// The length check above bounds data URI generation; no further
	// validation read is needed (bytes.Reader never fails).
	// Avoid per-request ExecAllocator leak: reuse shared allocator and create
	// isolated Browser contexts via chromedp.NewContext.
	var profile string

	if d.tmpDirSet {
		var err error

		profile, err = os.MkdirTemp(d.tmpDir, "chromedp-profile-")
		if err != nil {
			return nil, fmt.Errorf("local: user data dir: %w", err)
		}

		defer func() { _ = os.RemoveAll(profile) }()
	}

	rctx, rCancel := context.WithCancel(d.allocCtx)
	defer rCancel()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("local: render: %w", err)
	}

	stop := context.AfterFunc(ctx, rCancel)
	defer stop()

	ctx, tCancel := context.WithTimeout(rctx, d.timeout)
	defer tCancel()

	ctx, cCancel := chromedp.NewContext(ctx)
	defer cCancel()

	var buf []byte

	uri := "data:text/html;base64," + base64.StdEncoding.EncodeToString(source)

	tasks := chromedp.Tasks{chromedp.Navigate(uri), chromedp.WaitReady("body")}

	switch format {
	case document.FormatPDF:
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error

			buf, _, err = page.PrintToPDF().Do(ctx)

			return err
		}))
	case document.FormatPNG:
		tasks = append(tasks, chromedp.FullScreenshot(&buf, 100))
	case document.FormatJPG:
		q := int(d.quality)
		if q <= 0 {
			q = 80
		}

		if q >= 100 {
			q = 99
		}

		tasks = append(tasks, chromedp.FullScreenshot(&buf, q))
	}

	err := chromedp.Run(ctx, tasks...)
	if err != nil {
		return nil, fmt.Errorf("local: render: %w", err)
	}

	return buf, nil
}

// Close releases backend resources.
func (d *driver) Close() error {
	d.closeOnce.Do(d.allocCancel)

	return nil
}
