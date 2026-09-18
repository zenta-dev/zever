package local

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif" // register GIF decoder
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"

	"github.com/zenta-dev/zever/media"
	"github.com/zenta-dev/zever/media/ffmpeg"
)

type adapter struct {
	root        string
	baseURL     string
	maxDownload int64
	maxPixels   int64
	maxDuration time.Duration
	derivedDir  string
	derivedTTL  time.Duration
	ffmpeg      string
	ffprobe     string
	stop        chan struct{}
	closeOnce   sync.Once
}

var generateID = media.GenerateID

// localErr wraps err with the local adapter prefix.
func localErr(err error) error {
	return fmt.Errorf("local: %w", err)
}

// New creates a local media backend from the given Options.
func New(o media.Options) (media.Media, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}

	root := o.Root
	if root == "" {
		root = media.DefaultLocalRoot
	}

	baseURL := o.BaseURL
	if baseURL == "" {
		baseURL = media.DefaultLocalBaseURL
	}

	baseURL = strings.TrimRight(baseURL, "/")

	maxDownload := o.MaxDownloadBytes
	if maxDownload <= 0 {
		maxDownload = media.DefaultMaxDownloadBytes
	}

	maxPixels := o.MaxPixels
	if maxPixels <= 0 {
		maxPixels = media.DefaultMaxPixels
	}

	derivedTTL := o.DerivedTTL
	if derivedTTL <= 0 {
		derivedTTL = media.DefaultDerivedTTL
	}

	ffmpegBin := o.FFmpeg
	if ffmpegBin == "" {
		ffmpegBin = media.DefaultFFmpeg
	}

	ffprobeBin := o.FFProbe
	if ffprobeBin == "" {
		ffprobeBin = media.DefaultFFProbe
	}

	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("local: mkdir root: %w", err)
	}

	derivedDir := filepath.Join(root, "derived")

	if err := os.MkdirAll(derivedDir, 0o750); err != nil {
		return nil, fmt.Errorf("local: mkdir derived: %w", err)
	}

	a := &adapter{
		root:        root,
		baseURL:     baseURL,
		maxDownload: maxDownload,
		maxPixels:   maxPixels,
		maxDuration: o.MaxDuration,
		derivedDir:  derivedDir,
		derivedTTL:  derivedTTL,
		ffmpeg:      ffmpegBin,
		ffprobe:     ffprobeBin,
	}

	if derivedTTL > 0 {
		a.stop = make(chan struct{})
		go a.runJanitor()
	}

	return a, nil
}

func (a *adapter) Upload(_ context.Context, path string, data []byte, opts media.UploadOptions) (media.Asset, error) {
	id, err := generateID()
	if err != nil {
		return media.Asset{}, fmt.Errorf("local: generate id: %w", err)
	}

	ext := filepath.Ext(path)
	if ext == "" {
		ext = media.ExtForContentType(opts.ContentType)
	}

	dest := filepath.Join(a.root, id+ext)

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil { //nolint:gosec // dest is confined to root via hex id
		return media.Asset{}, fmt.Errorf("local: mkdir: %w", err)
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil { //nolint:gosec // media file, not secret
		return media.Asset{}, fmt.Errorf("local: write: %w", err)
	}

	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	return media.Asset{
		ID:          id,
		URL:         a.baseURL + "/" + id + ext,
		ContentType: ct,
		Size:        int64(len(data)),
	}, nil
}

func (a *adapter) Download(_ context.Context, id string) ([]byte, error) {
	src, err := a.findFile(id)
	if err != nil {
		return nil, err
	}

	maxVal := a.maxDownload
	if maxVal <= 0 {
		maxVal = media.DefaultMaxDownloadBytes
	}

	f, err := os.Open(src) //nolint:gosec // path validated via findFile/ValidHexID
	if err != nil {
		return nil, fmt.Errorf("local: open: %w", err)
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxVal+1))
	if err != nil {
		return nil, fmt.Errorf("local: read: %w", err)
	}

	if int64(len(data)) > maxVal {
		return nil, localErr(&media.SizeLimitError{Size: len(data), Limit: int(maxVal)})
	}

	return data, nil
}

func (a *adapter) DownloadRange(_ context.Context, id string, offset, length int64) ([]byte, error) {
	if offset < 0 {
		return nil, localErr(&media.InvalidRangeError{Offset: offset, Length: length})
	}

	if length < 0 {
		return nil, localErr(&media.InvalidRangeError{Offset: offset, Length: length})
	}

	src, err := a.findFile(id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(src) //nolint:gosec // path validated via findFile/ValidHexID
	if err != nil {
		return nil, fmt.Errorf("local: read: %w", err)
	}

	size := int64(len(data))

	if offset >= size {
		return nil, localErr(&media.InvalidRangeError{Offset: offset, Length: length, Size: size})
	}

	end := size
	if length > 0 && length < size-offset {
		end = offset + length
	}

	out := data[offset:end]

	maxVal := a.maxDownload
	if maxVal <= 0 {
		maxVal = media.DefaultMaxDownloadBytes
	}

	if int64(len(out)) > maxVal {
		return nil, localErr(&media.SizeLimitError{Size: len(out), Limit: int(maxVal)})
	}

	return out, nil
}

func (a *adapter) Delete(_ context.Context, id string) error {
	if !media.ValidHexID(id) {
		return localErr(&media.InvalidIDError{ID: id})
	}

	matches, err := filepath.Glob(filepath.Join(a.root, id+".*"))
	if err != nil {
		return fmt.Errorf("local: glob: %w", err)
	}

	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			return fmt.Errorf("local: remove: %w", err)
		}
	}

	return nil
}

func (a *adapter) Stat(_ context.Context, id string) (media.Info, error) {
	src, err := a.findFile(id)
	if err != nil {
		return media.Info{}, err
	}

	fi, err := os.Stat(src)
	if err != nil {
		return media.Info{}, fmt.Errorf("local: stat: %w", err)
	}

	return media.Info{
		ID:          id,
		Size:        fi.Size(),
		ContentType: media.ContentTypeForExt(filepath.Ext(src)),
	}, nil
}

func (a *adapter) Probe(ctx context.Context, id string) (media.Probe, error) {
	src, err := a.findFile(id)
	if err != nil {
		return media.Probe{}, err
	}

	ext := filepath.Ext(src)

	kind, ok := media.KindForExt(ext)
	if !ok {
		return media.Probe{}, localErr(&media.UnsupportedFormatError{Format: ext})
	}

	if kind != media.KindImage {
		return a.probeAV(ctx, src)
	}

	f, err := os.Open(src) //nolint:gosec // path validated via findFile/ValidHexID
	if err != nil {
		return media.Probe{}, fmt.Errorf("local: open: %w", err)
	}

	cfg, _, err := image.DecodeConfig(f)
	_ = f.Close()

	if err != nil {
		return media.Probe{}, fmt.Errorf("local: probe: %w: %w", err, media.ErrProbeFailed)
	}

	return media.Probe{
		Kind:   media.KindImage,
		Format: strings.ToLower(strings.TrimPrefix(ext, ".")),
		Width:  cfg.Width,
		Height: cfg.Height,
	}, nil
}

func (a *adapter) Transform(ctx context.Context, id string, ops media.TransformOps) (string, error) {
	src, err := a.findFile(id)
	if err != nil {
		return "", err
	}

	ext := filepath.Ext(src)

	kind, ok := media.KindForExt(ext)
	if !ok {
		return "", localErr(&media.UnsupportedFormatError{Format: ext})
	}

	if kind != media.KindImage {
		return a.avTransform(ctx, src, id, kind, ops)
	}

	w, h, err := parseDims(ops.Width, ops.Height)
	if err != nil {
		return "", err
	}

	img, err := a.loadImage(src)
	if err != nil {
		return "", err
	}

	result := applyResize(img, w, h)

	outPath, urlPath, err := a.prepareOutput(src, ops.Format)
	if err != nil {
		return "", err
	}

	encoder, err := selectEncoder(filepath.Ext(outPath), ops.Quality)
	if err != nil {
		return "", err
	}

	if err := imgio.Save(outPath, result, encoder); err != nil {
		return "", fmt.Errorf("local: save: %w", err)
	}

	return a.baseURL + "/" + urlPath, nil
}

// isThumbFormat reports whether format names an image container usable for
// video thumbnail extraction.
func isThumbFormat(format string) bool {
	kind, ok := media.KindForExt(format)

	return ok && kind == media.KindImage
}

// avTransform derives an audio/video variant via ffmpeg. Output names are
// deterministic (id + variant + ext) so identical requests are idempotent.
func (a *adapter) avTransform(ctx context.Context, src, id string, kind media.MediaKind, ops media.TransformOps) (string, error) {
	w, h, err := parseDims(ops.Width, ops.Height)
	if err != nil {
		return "", err
	}

	if ops.Bitrate < 0 {
		return "", localErr(&media.InvalidTransformError{Reason: "bitrate must be >= 0"})
	}

	if ops.CRF < 0 || ops.CRF > 51 {
		return "", localErr(&media.InvalidTransformError{Reason: "crf must be between 0 and 51"})
	}

	format := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ops.Format)), ".")
	if format == "" {
		return "", localErr(&media.InvalidTransformError{Reason: "format is required for audio/video"})
	}

	switch {
	case kind == media.KindAudio && !ffmpeg.IsAudioFormat(format):
		return "", localErr(&media.InvalidTransformError{Reason: "format " + format + " is not an audio format"})
	case kind == media.KindVideo && !ffmpeg.IsVideoFormat(format) && !isThumbFormat(format):
		return "", localErr(&media.InvalidTransformError{Reason: "format " + format + " is not a video or image format"})
	}

	thumb := isThumbFormat(format)

	if ops.Offset < 0 {
		return "", localErr(&media.InvalidTransformError{Reason: "offset must be >= 0"})
	}

	if ops.Offset > 0 && (!thumb || kind != media.KindVideo) {
		return "", localErr(&media.InvalidTransformError{Reason: "offset is only valid for video thumbnails"})
	}

	ffmpegPath, err := ffmpeg.LookPath(a.ffmpeg)
	if err != nil {
		return "", localErr(media.ErrToolMissing)
	}

	if a.maxDuration > 0 {
		probe, err := a.probeAV(ctx, src)
		if err != nil {
			return "", err
		}

		if probe.Duration > a.maxDuration {
			return "", localErr(media.ErrDurationExceeded)
		}
	}

	parts := make([]string, 0, 5)
	if w > 0 {
		parts = append(parts, "w"+strconv.Itoa(w))
	}

	if h > 0 {
		parts = append(parts, "h"+strconv.Itoa(h))
	}

	if ops.Bitrate > 0 {
		parts = append(parts, "br"+strconv.Itoa(ops.Bitrate))
	}

	if ops.CRF > 0 {
		parts = append(parts, "crf"+strconv.Itoa(ops.CRF))
	}

	if thumb {
		parts = append(parts, "thumb")
	}

	outExt := "." + format

	outName := id + outExt
	if len(parts) > 0 {
		outName = id + "-" + strings.Join(parts, "-") + outExt
	}

	outPath := filepath.Join(a.derivedDir, outName)

	args := ffmpeg.BuildArgs(src, outPath, ffmpeg.Spec{
		Kind:    kind,
		Format:  format,
		Width:   w,
		Height:  h,
		Bitrate: ops.Bitrate,
		CRF:     ops.CRF,
		Quality: ops.Quality,
		Offset:  ops.Offset,
	})

	if err := ffmpeg.RunTranscode(ctx, ffmpegPath, args); err != nil {
		return "", fmt.Errorf("local: transcode: %w: %w", err, media.ErrTranscodeFailed)
	}

	return a.baseURL + "/" + filepath.Base(a.derivedDir) + "/" + outName, nil
}

// probeAV runs ffprobe and converts the result, mapping tool failures to media sentinels.
func (a *adapter) probeAV(ctx context.Context, src string) (media.Probe, error) {
	ffprobePath, err := ffmpeg.LookPath(a.ffprobe)
	if err != nil {
		return media.Probe{}, localErr(media.ErrToolMissing)
	}

	res, err := ffmpeg.Probe(ctx, ffprobePath, src)
	if err != nil {
		return media.Probe{}, fmt.Errorf("local: probe: %w: %w", err, media.ErrProbeFailed)
	}

	probe := res.ToProbe()
	if probe.Kind == "" {
		return media.Probe{}, fmt.Errorf("local: probe: %w", media.ErrProbeFailed)
	}

	return probe, nil
}

// Close releases backend resources.
func (a *adapter) Close() error {
	if a.stop != nil {
		a.closeOnce.Do(func() { close(a.stop) })
	}

	return nil
}

func (a *adapter) loadImage(src string) (image.Image, error) {
	maxBytes := a.maxDownload
	if maxBytes <= 0 {
		maxBytes = media.DefaultMaxDownloadBytes
	}

	f, err := os.Open(src) //nolint:gosec // path validated via findFile/ValidHexID
	if err != nil {
		return nil, fmt.Errorf("local: open: %w", err)
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("local: read: %w", err)
	}

	if int64(len(data)) > maxBytes {
		return nil, localErr(&media.SizeLimitError{Size: len(data), Limit: int(maxBytes)})
	}

	maxPixels := a.maxPixels
	if maxPixels <= 0 {
		maxPixels = media.DefaultMaxPixels
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("local: load: %w", err)
	}

	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("local: image dimensions %dx%d exceed max pixels %d", cfg.Width, cfg.Height, maxPixels)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("local: load: %w", err)
	}

	return img, nil
}

func applyResize(img image.Image, w, h int) image.Image {
	if w <= 0 && h <= 0 {
		return img
	}

	srcW, srcH := img.Bounds().Dx(), img.Bounds().Dy()

	if w <= 0 {
		w = max(1, h*srcW/srcH)
	}

	if h <= 0 {
		h = max(1, w*srcH/srcW)
	}

	return transform.Resize(img, w, h, transform.Linear)
}

func parseDims(width, height string) (int, int, error) {
	w, err := parseDim(width, "width")
	if err != nil {
		return 0, 0, err
	}

	h, err := parseDim(height, "height")
	if err != nil {
		return 0, 0, err
	}

	return w, h, nil
}

func parseDim(s, name string) (int, error) {
	if s == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, localErr(&media.InvalidTransformError{Reason: name + " must be a non-negative integer"})
	}

	return n, nil
}

func (a *adapter) prepareOutput(src, format string) (string, string, error) {
	outID, err := generateID()
	if err != nil {
		return "", "", fmt.Errorf("local: generate id: %w", err)
	}

	ext := filepath.Ext(src)
	if format != "" {
		ext, err = formatExt(format)
		if err != nil {
			return "", "", err
		}
	}

	dir := a.root
	urlPath := outID + ext

	if a.derivedDir != "" {
		dir = a.derivedDir

		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", "", fmt.Errorf("local: mkdir derived: %w", err)
		}

		urlPath = filepath.Base(a.derivedDir) + "/" + outID + ext
	}

	outPath := filepath.Join(dir, outID+ext)

	return outPath, urlPath, nil
}

func (a *adapter) findFile(id string) (string, error) {
	if !media.ValidHexID(id) {
		return "", localErr(&media.InvalidIDError{ID: id})
	}

	matches, err := filepath.Glob(filepath.Join(a.root, id+".*"))
	if err != nil {
		return "", fmt.Errorf("local: glob: %w", err)
	}

	if len(matches) == 0 {
		return "", localErr(&media.NotFoundError{ID: id})
	}

	return matches[0], nil
}

func (a *adapter) cleanDerived() {
	if a.derivedDir == "" || a.derivedTTL <= 0 {
		return
	}

	entries, err := os.ReadDir(a.derivedDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-a.derivedTTL)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		info, err := os.Stat(filepath.Join(a.derivedDir, e.Name()))
		if err != nil {
			continue
		}

		if info.ModTime().After(cutoff) {
			continue
		}

		if err := os.Remove(filepath.Join(a.derivedDir, e.Name())); err != nil && !os.IsNotExist(err) {
			continue
		}
	}
}

func (a *adapter) runJanitor() {
	if a.derivedTTL <= 0 {
		return
	}

	ticker := time.NewTicker(a.derivedTTL / 2)
	defer ticker.Stop()

	a.cleanDerived()

	for {
		select {
		case <-ticker.C:
			a.cleanDerived()
		case <-a.stop:
			return
		}
	}
}

func selectEncoder(ext string, quality int) (imgio.Encoder, error) {
	switch strings.TrimPrefix(ext, ".") {
	case "png":
		return imgio.PNGEncoder(), nil
	case "webp":
		return imgio.WEBPEncoder(nil), nil
	case "jpg", "jpeg":
		return imgio.JPEGEncoder(clampQuality(quality)), nil
	default:
		return nil, localErr(&media.UnsupportedFormatError{Format: ext})
	}
}

func clampQuality(q int) int {
	if q <= 0 {
		return 80
	}

	if q > 100 {
		return 100
	}

	return q
}

func formatExt(f string) (string, error) {
	switch strings.ToLower(f) {
	case "jpg", "jpeg", "png", "webp":
		return "." + strings.ToLower(f), nil
	default:
		return "", localErr(&media.UnsupportedFormatError{Format: f})
	}
}
