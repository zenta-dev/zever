package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // registers GIF decoding for Probe and Transform.
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/shared/codec"
	"github.com/zenta-dev/zever/shared/s3opts"
)

// driver stores media assets in an S3-compatible bucket.
type driver struct {
	client      *s3sdk.Client
	presigner   *s3sdk.PresignClient
	bucket      string
	baseURL     string
	maxDownload int64
	maxPixels   int64
	maxDuration time.Duration
	presignTTL  time.Duration
	ffmpeg      string
	ffprobe     string
}

// generateID is a seam for deterministic tests.
var generateID = media.GenerateID

// mkdirTemp and writeFile are seams for deterministic temp-dir failure tests.
var (
	mkdirTemp = os.MkdirTemp
	writeFile = os.WriteFile
)

// New validates opts and returns a Media backed by S3.
//
// Access keys are always required, even for S3-compatible backends like MinIO:
// anonymous SDK clients cannot sign requests. BaseURL only shapes the Asset
// URLs returned to callers and is never used as the SDK endpoint; use Endpoint
// for custom backends. Uploads use a single PUT, so individual assets are
// limited to 5GB.
func New(o media.Options) (media.Media, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	cfg := s3opts.Options{
		Endpoint:        o.Endpoint,
		Region:          o.Region,
		Bucket:          o.Bucket,
		AccessKeyID:     o.AccessKeyID,
		SecretAccessKey: o.SecretAccessKey,
	}.WithDefaults(media.DefaultS3Region)

	if err := cfg.ValidateBucket(); err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	if err := cfg.ValidateCredentials(); err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	maxDownload := o.MaxDownloadBytes
	if maxDownload == 0 {
		maxDownload = media.DefaultMaxDownloadBytes
	}

	maxPixels := o.MaxPixels
	if maxPixels == 0 {
		maxPixels = media.DefaultMaxPixels
	}

	ttl := o.PresignTTL
	if ttl == 0 {
		ttl = media.DefaultPresignTTL
	}

	ffmpeg := o.FFmpeg
	if ffmpeg == "" {
		ffmpeg = media.DefaultFFmpeg
	}

	ffprobe := o.FFProbe
	if ffprobe == "" {
		ffprobe = media.DefaultFFProbe
	}

	client, presigner, err := s3opts.NewClient(context.Background(), "s3", cfg, "")
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	return &driver{
		client:      client,
		presigner:   presigner,
		bucket:      cfg.Bucket,
		baseURL:     strings.TrimRight(o.BaseURL, "/"),
		maxDownload: maxDownload,
		maxPixels:   maxPixels,
		maxDuration: o.MaxDuration,
		presignTTL:  ttl,
		ffmpeg:      ffmpeg,
		ffprobe:     ffprobe,
	}, nil
}

// isNoSuchKey reports whether err is an S3 missing-key error. The v2 SDK
// exposes the code via smithy.APIError.ErrorCode (there is no Code method);
// GetObject misses surface as NoSuchKey while HeadObject misses surface as
// NotFound, so both map to not-found.
func isNoSuchKey(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	code := apiErr.ErrorCode()
	return code == "NoSuchKey" || code == "NotFound"
}

// findKey resolves an asset id to its full object key via a prefix listing.
// IDs are hex, so the prefix cannot escape into other assets.
func (d *driver) findKey(ctx context.Context, id string) (string, error) {
	out, err := d.client.ListObjectsV2(ctx, &s3sdk.ListObjectsV2Input{
		Bucket:  aws.String(d.bucket),
		Prefix:  aws.String(id),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return "", fmt.Errorf("s3: list: %w", err)
	}

	if len(out.Contents) == 0 {
		return "", fmt.Errorf("s3: %w", media.NotFoundError{ID: id})
	}

	return aws.ToString(out.Contents[0].Key), nil
}

// assetURL returns the access URL for key: the public base URL when set,
// otherwise a presigned GET URL.
func (d *driver) assetURL(ctx context.Context, key string) (string, error) {
	if d.baseURL != "" {
		return d.baseURL + "/" + key, nil
	}

	req, err := d.presigner.PresignGetObject(ctx, &s3sdk.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}, func(o *s3sdk.PresignOptions) {
		o.Expires = d.presignTTL
	})
	if err != nil {
		return "", fmt.Errorf("s3: presign: %w", err)
	}

	return req.URL, nil
}

// readCapped drains body up to limit bytes.
func readCapped(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > limit {
		return nil, media.SizeLimitError{Size: len(data), Limit: int(limit)}
	}

	return data, nil
}

// put stores data under key with content type ct.
func (d *driver) put(ctx context.Context, key string, data []byte, ct string) error {
	_, err := d.client.PutObject(ctx, &s3sdk.PutObjectInput{
		Bucket:      aws.String(d.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(ct),
	})
	if err != nil {
		return fmt.Errorf("s3: put: %w", err)
	}

	return nil
}

// fetchCapped downloads key, mapping a lost race to NotFound and capping bytes.
func (d *driver) fetchCapped(ctx context.Context, key, id string) ([]byte, error) {
	out, err := d.client.GetObject(ctx, &s3sdk.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNoSuchKey(err) {
			return nil, fmt.Errorf("s3: %w", media.NotFoundError{ID: id})
		}

		return nil, fmt.Errorf("s3: get: %w", err)
	}
	defer out.Body.Close()

	data, err := readCapped(out.Body, d.maxDownload)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	return data, nil
}

// headSize returns the object size, mapping misses to NotFound.
func (d *driver) headSize(ctx context.Context, key, id string) (int64, error) {
	out, err := d.client.HeadObject(ctx, &s3sdk.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNoSuchKey(err) {
			return 0, fmt.Errorf("s3: %w", media.NotFoundError{ID: id})
		}

		return 0, fmt.Errorf("s3: head: %w", err)
	}

	return aws.ToInt64(out.ContentLength), nil
}

// Upload stores data and returns its Asset.
func (d *driver) Upload(ctx context.Context, path string, data []byte, opts media.UploadOptions) (media.Asset, error) {
	id, err := generateID()
	if err != nil {
		return media.Asset{}, fmt.Errorf("s3: generate id: %w", err)
	}

	ext := filepath.Ext(path)
	if ext == "" {
		ext = media.ExtForContentType(opts.ContentType)
	}

	ct := opts.ContentType
	if ct == "" {
		ct = media.ContentTypeForExt(ext)
	}

	key := id + ext
	if err := d.put(ctx, key, data, ct); err != nil {
		return media.Asset{}, err
	}

	assetURL, urlErr := d.assetURL(ctx, key)
	if urlErr != nil {
		return media.Asset{}, urlErr
	}

	return media.Asset{ID: id, URL: assetURL, ContentType: ct, Size: int64(len(data))}, nil
}

// Download fetches asset id.
func (d *driver) Download(ctx context.Context, id string) ([]byte, error) {
	key, err := d.findKey(ctx, id)
	if err != nil {
		return nil, err
	}

	return d.fetchCapped(ctx, key, id)
}

// DownloadRange fetches [offset, offset+length) of asset id. A length of zero
// fetches to the end; negative offsets and lengths are invalid.
func (d *driver) DownloadRange(ctx context.Context, id string, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 {
		return nil, media.InvalidRangeError{Offset: offset, Length: length}
	}

	key, err := d.findKey(ctx, id)
	if err != nil {
		return nil, err
	}

	size, err := d.headSize(ctx, key, id)
	if err != nil {
		return nil, err
	}

	if offset >= size {
		return nil, media.InvalidRangeError{Offset: offset, Length: length, Size: size}
	}

	rng := "bytes=" + strconv.FormatInt(offset, 10) + "-"
	if length > 0 {
		end := size - 1
		if e := offset + length - 1; e < end {
			end = e
		}
		rng = "bytes=" + strconv.FormatInt(offset, 10) + "-" + strconv.FormatInt(end, 10)
	}

	out, err := d.client.GetObject(ctx, &s3sdk.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
		Range:  aws.String(rng),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidRange" {
			return nil, media.InvalidRangeError{Offset: offset, Length: length, Size: size}
		}

		if isNoSuchKey(err) {
			return nil, fmt.Errorf("s3: %w", media.NotFoundError{ID: id})
		}

		return nil, fmt.Errorf("s3: get: %w", err)
	}
	defer out.Body.Close()

	data, err := readCapped(out.Body, d.maxDownload)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	return data, nil
}

// Delete removes asset id. Deleting a missing asset is a no-op.
func (d *driver) Delete(ctx context.Context, id string) error {
	key, err := d.findKey(ctx, id)
	if err != nil {
		if errors.Is(err, media.ErrNotFound) {
			return nil
		}

		return err
	}

	_, err = d.client.DeleteObject(ctx, &s3sdk.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete: %w", err)
	}

	return nil
}

// Stat describes asset id.
func (d *driver) Stat(ctx context.Context, id string) (media.Info, error) {
	key, err := d.findKey(ctx, id)
	if err != nil {
		return media.Info{}, err
	}

	out, err := d.client.HeadObject(ctx, &s3sdk.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNoSuchKey(err) {
			return media.Info{}, fmt.Errorf("s3: %w", media.NotFoundError{ID: id})
		}

		return media.Info{}, fmt.Errorf("s3: head: %w", err)
	}

	ct := aws.ToString(out.ContentType)
	if ct == "" {
		ct = media.ContentTypeForExt(filepath.Ext(key))
	}

	return media.Info{ID: id, Size: aws.ToInt64(out.ContentLength), ContentType: ct}, nil
}

// ffprobeOutput is the subset of ffprobe JSON output we read.
type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []ffStream `json:"streams"`
}

// ffStream is a single ffprobe stream entry.
type ffStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// ffprobeCodec decodes ffprobe JSON output into ffprobeOutput. ffprobeOutput
// has no custom marshaler, so migrating it to codec.JSONCodec (encoding/json/v2)
// carries no interop risk.
var ffprobeCodec = codec.JSONCodec[ffprobeOutput]{}

// stageTemp writes data to a private temp file for external tools.
func stageTemp(ext string, data []byte) (string, string, error) {
	dir, err := mkdirTemp("", "medias3-")
	if err != nil {
		return "", "", fmt.Errorf("s3: temp: %w", err)
	}

	path := filepath.Join(dir, "in"+ext)
	if err := writeFile(path, data, 0o600); err != nil {
		os.RemoveAll(dir)
		return "", "", fmt.Errorf("s3: temp: %w", err)
	}

	return dir, path, nil
}

// runFFProbe executes ffprobe and parses its JSON output.
func runFFProbe(ctx context.Context, bin, file string) (ffprobeOutput, error) {
	var parsed ffprobeOutput

	if _, err := exec.LookPath(bin); err != nil {
		return parsed, fmt.Errorf("s3: %w (%s)", media.ErrToolMissing, bin)
	}

	raw, err := exec.CommandContext(ctx, bin, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", file).Output()
	if err != nil {
		return parsed, fmt.Errorf("s3: probe: %w", media.ErrProbeFailed)
	}

	parsed, err = ffprobeCodec.Decode(raw)
	if err != nil {
		return parsed, fmt.Errorf("s3: probe: %w", media.ErrProbeFailed)
	}

	return parsed, nil
}

// ffDuration converts the ffprobe duration string to a Duration.
func ffDuration(fp ffprobeOutput) (time.Duration, error) {
	if fp.Format.Duration == "" {
		return 0, nil
	}

	secs, err := strconv.ParseFloat(fp.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("s3: probe: %w", media.ErrProbeFailed)
	}

	return time.Duration(secs * float64(time.Second)), nil
}

// checkDuration enforces the max-duration gate.
func (d *driver) checkDuration(dur time.Duration) error {
	if d.maxDuration > 0 && dur > d.maxDuration {
		return fmt.Errorf("s3: %w", media.ErrDurationExceeded)
	}

	return nil
}

// probeImage detects still-image properties.
func (d *driver) probeImage(data []byte, ext string) (media.Probe, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return media.Probe{}, fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(ext, ".")})
	}

	if d.maxPixels > 0 && int64(cfg.Width)*int64(cfg.Height) > d.maxPixels {
		return media.Probe{}, fmt.Errorf("s3: %w", media.SizeLimitError{Size: cfg.Width * cfg.Height, Limit: int(d.maxPixels)})
	}

	return media.Probe{Kind: media.KindImage, Format: strings.TrimPrefix(ext, "."), Width: cfg.Width, Height: cfg.Height}, nil
}

// probeAV detects audio/video properties via ffprobe.
func (d *driver) probeAV(ctx context.Context, data []byte, ext string, kind media.MediaKind) (media.Probe, error) {
	dir, in, err := stageTemp(ext, data)
	if err != nil {
		return media.Probe{}, err
	}
	defer os.RemoveAll(dir)

	fp, err := runFFProbe(ctx, d.ffprobe, in)
	if err != nil {
		return media.Probe{}, err
	}

	p := media.Probe{Kind: kind, Format: strings.TrimPrefix(ext, ".")}
	for _, s := range fp.Streams {
		switch s.CodecType {
		case "audio":
			p.AudioCodec = s.CodecName
		case "video":
			p.Kind = media.KindVideo
			p.VideoCodec = s.CodecName
			p.Width = s.Width
			p.Height = s.Height
		}
	}

	dur, err := ffDuration(fp)
	if err != nil {
		return media.Probe{}, err
	}
	p.Duration = dur

	if err := d.checkDuration(p.Duration); err != nil {
		return media.Probe{}, err
	}

	return p, nil
}

// Probe detects the media properties of asset id.
func (d *driver) Probe(ctx context.Context, id string) (media.Probe, error) {
	key, err := d.findKey(ctx, id)
	if err != nil {
		return media.Probe{}, err
	}

	ext := strings.ToLower(filepath.Ext(key))
	kind, ok := media.KindForExt(ext)
	if !ok {
		return media.Probe{}, fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(ext, ".")})
	}

	data, err := d.fetchCapped(ctx, key, id)
	if err != nil {
		return media.Probe{}, err
	}

	if kind == media.KindImage {
		return d.probeImage(data, ext)
	}

	return d.probeAV(ctx, data, ext, kind)
}

// isIdentity reports whether ops is the zero (identity) transform.
func isIdentity(ops media.TransformOps) bool {
	return ops.Width == "" && ops.Height == "" && ops.Format == "" && ops.Quality == 0 &&
		ops.Bitrate == 0 && ops.CRF == 0 && ops.Offset == 0
}

// targetSize resolves Width/Height strings to pixel dims, scaling
// proportionally when only one side is set.
func targetSize(sw, sh int, wStr, hStr string) (int, int, error) {
	if wStr == "" && hStr == "" {
		return sw, sh, nil
	}

	var tw, th int
	if wStr != "" {
		w, err := strconv.Atoi(wStr)
		if err != nil || w <= 0 {
			return 0, 0, fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "invalid width"})
		}
		tw = w
	}
	if hStr != "" {
		h, err := strconv.Atoi(hStr)
		if err != nil || h <= 0 {
			return 0, 0, fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "invalid height"})
		}
		th = h
	}
	if tw == 0 {
		tw = sw * th / sh
	}
	if th == 0 {
		th = sh * tw / sw
	}

	return tw, th, nil
}

// resizeNN scales src to w*h with nearest-neighbor sampling.
func resizeNN(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sdx, sdy := sb.Dx(), sb.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(sb.Min.X+x*sdx/w, sb.Min.Y+y*sdy/h))
		}
	}

	return dst
}

// transformImage resizes or converts an image and stores the derived variant.
func (d *driver) transformImage(ctx context.Context, id, key, ext string, ops media.TransformOps) (string, error) {
	if ops.Bitrate != 0 || ops.CRF != 0 || ops.Offset != 0 {
		return "", fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "bitrate, crf and offset apply to audio/video only"})
	}

	if ops.Quality < 0 || ops.Quality > 100 {
		return "", fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "quality must be between 1 and 100"})
	}

	outExt := ext
	if ops.Format != "" {
		outExt = "." + strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ops.Format), "."))
	}
	if outKind, ok := media.KindForExt(outExt); !ok {
		return "", fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(outExt, ".")})
	} else if outKind != media.KindImage {
		return "", fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "cannot transform image to " + outExt})
	}

	data, err := d.fetchCapped(ctx, key, id)
	if err != nil {
		return "", err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(ext, ".")})
	}

	sb := img.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if d.maxPixels > 0 && int64(sw)*int64(sh) > d.maxPixels {
		return "", fmt.Errorf("s3: %w", media.SizeLimitError{Size: sw * sh, Limit: int(d.maxPixels)})
	}

	tw, th, err := targetSize(sw, sh, ops.Width, ops.Height)
	if err != nil {
		return "", err
	}
	if tw != sw || th != sh {
		img = resizeNN(img, tw, th)
	}

	quality := ops.Quality
	if quality == 0 {
		quality = 80
	}

	var buf bytes.Buffer
	switch outExt {
	case ".jpg", ".jpeg":
		// Encoding a decoded image to a memory buffer cannot fail.
		_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case ".png":
		// Encoding a decoded image to a memory buffer cannot fail.
		_ = png.Encode(&buf, img)
	default:
		return "", fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(outExt, ".")})
	}

	variant := strconv.Itoa(tw) + "x" + strconv.Itoa(th) + "-q" + strconv.Itoa(quality)
	if ops.Format != "" {
		variant += "-" + strings.TrimPrefix(outExt, ".")
	}
	derived := id + "-" + variant + outExt

	if err := d.put(ctx, derived, buf.Bytes(), media.ContentTypeForExt(outExt)); err != nil {
		return "", err
	}

	return d.assetURL(ctx, derived)
}

// formatOffset renders a seek offset in ffmpeg seconds notation.
func formatOffset(offset time.Duration) string {
	return strconv.FormatFloat(offset.Seconds(), 'f', 3, 64)
}

// transformAV transcodes audio/video via ffmpeg and stores the derived variant.
func (d *driver) transformAV(ctx context.Context, id, key, ext string, kind media.MediaKind, ops media.TransformOps) (string, error) {
	if ops.Width != "" || ops.Height != "" || ops.Quality != 0 {
		return "", fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "width, height and quality apply to images only"})
	}

	if ops.Bitrate < 0 || ops.CRF < 0 || ops.Offset < 0 {
		return "", fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "bitrate, crf and offset must be >= 0"})
	}

	outExt := ext
	if ops.Format != "" {
		outExt = "." + strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ops.Format), "."))
	}
	if outKind, ok := media.KindForExt(outExt); !ok {
		return "", fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(outExt, ".")})
	} else if outKind != kind {
		return "", fmt.Errorf("s3: %w", media.InvalidTransformError{Reason: "cannot transform " + string(kind) + " to " + outExt})
	}

	if _, err := exec.LookPath(d.ffmpeg); err != nil {
		return "", fmt.Errorf("s3: %w (%s)", media.ErrToolMissing, d.ffmpeg)
	}

	data, err := d.fetchCapped(ctx, key, id)
	if err != nil {
		return "", err
	}

	dir, in, err := stageTemp(ext, data)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	if d.maxDuration > 0 {
		fp, probeErr := runFFProbe(ctx, d.ffprobe, in)
		if probeErr != nil {
			return "", probeErr
		}
		dur, durErr := ffDuration(fp)
		if durErr != nil {
			return "", durErr
		}
		if err = d.checkDuration(dur); err != nil {
			return "", err
		}
	}

	args := []string{"-y", "-i", in}
	if ops.Bitrate > 0 {
		args = append(args, "-b:a", strconv.Itoa(ops.Bitrate)+"k")
	}
	if ops.CRF > 0 {
		args = append(args, "-crf", strconv.Itoa(ops.CRF))
	}
	if ops.Offset > 0 {
		args = append(args, "-ss", formatOffset(ops.Offset))
	}
	out := filepath.Join(dir, "out"+outExt)
	args = append(args, out)

	//nolint:gosec // ffmpeg binary is caller-configured; argv built from validated ops.
	if err = exec.CommandContext(ctx, d.ffmpeg, args...).Run(); err != nil {
		return "", fmt.Errorf("s3: transcode: %w", media.ErrTranscodeFailed)
	}

	//nolint:gosec // out is staged under os.MkdirTemp by this function.
	outData, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("s3: read: %w", err)
	}

	variant := "t"
	if ops.Bitrate > 0 {
		variant += "-b" + strconv.Itoa(ops.Bitrate)
	}
	if ops.CRF > 0 {
		variant += "-crf" + strconv.Itoa(ops.CRF)
	}
	if ops.Offset > 0 {
		variant += "-ss" + formatOffset(ops.Offset)
	}
	if ops.Format != "" {
		variant += "-" + strings.TrimPrefix(outExt, ".")
	}
	derived := id + "-" + variant + outExt

	if err := d.put(ctx, derived, outData, media.ContentTypeForExt(outExt)); err != nil {
		return "", err
	}

	return d.assetURL(ctx, derived)
}

// Transform derives a variant of asset id per ops and returns its URL.
func (d *driver) Transform(ctx context.Context, id string, ops media.TransformOps) (string, error) {
	key, err := d.findKey(ctx, id)
	if err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(key))
	kind, ok := media.KindForExt(ext)
	if !ok {
		return "", fmt.Errorf("s3: %w", media.UnsupportedFormatError{Format: strings.TrimPrefix(ext, ".")})
	}

	if isIdentity(ops) {
		return d.assetURL(ctx, key)
	}

	if kind == media.KindImage {
		return d.transformImage(ctx, id, key, ext, ops)
	}

	return d.transformAV(ctx, id, key, ext, kind, ops)
}

// Close releases backend resources.
func (d *driver) Close() error {
	return nil
}
