package osm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/shared/codec"
	endpointpkg "github.com/zenta-dev/zever/shared/endpoint"
	"github.com/zenta-dev/zever/shared/httpclient"
)

const defaultEndpoint = "https://nominatim.openstreetmap.org"
const defaultMaxBody = 1 << 20
const defaultTimeout = 10 * time.Second

// defaultMinRequestInterval paces outbound requests to Nominatim's public
// instance at no more than 1 request/second, per its usage policy
// (https://operations.osmfoundation.org/policies/nominatim/): "An absolute
// maximum of 1 request per second." Exceeding it risks the caller's IP
// being blocked, taking down geocoding for everyone sharing that address.
const defaultMinRequestInterval = time.Second

type osmGeo struct {
	endpoint  string
	baseURL   *url.URL
	client    *http.Client
	userAgent string
	maxBody   int64

	// minInterval paces requests (see defaultMinRequestInterval); zero
	// (the osmGeo{} literal some tests construct directly) disables pacing.
	minInterval time.Duration
	paceMu      sync.Mutex
	nextAllowed time.Time
}

// New creates an OSM Nominatim-backed geo.Geo.
func New(opts geo.Options) (geo.Geo, error) {
	if strings.TrimSpace(opts.UserAgent) == "" {
		return nil, fmt.Errorf("geo: osm: %w: user_agent is required", geo.ErrInvalidOptions)
	}

	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = opts.BaseURL
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	if err := validateEndpoint(endpoint, opts.AllowInsecure); err != nil {
		return nil, err
	}

	endpoint = strings.TrimSuffix(endpoint, "/")

	u, _ := url.Parse(endpoint)

	maxBody := int64(opts.MaxResponseBody)
	if maxBody == 0 {
		maxBody = defaultMaxBody
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &osmGeo{
		endpoint:    endpoint,
		baseURL:     u,
		client:      httpclient.NewClient(timeout),
		userAgent:   opts.UserAgent,
		maxBody:     maxBody,
		minInterval: defaultMinRequestInterval,
	}, nil
}

func validateEndpoint(endpoint string, allowInsecure bool) error {
	if _, err := endpointpkg.ValidateURL(endpoint, endpointpkg.WithAllowInsecure(allowInsecure)); err != nil {
		switch {
		case errors.Is(err, endpointpkg.ErrParse),
			errors.Is(err, endpointpkg.ErrEmpty),
			errors.Is(err, endpointpkg.ErrNoScheme),
			errors.Is(err, endpointpkg.ErrNoHost):
			return fmt.Errorf("geo: osm: %w: endpoint must be a valid URL", geo.ErrInvalidOptions)
		default:
			return fmt.Errorf("geo: osm: %w: endpoint must use https", geo.ErrInvalidOptions)
		}
	}
	return nil
}

type nominatimResult struct {
	PlaceID     int               `json:"place_id"`
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	DisplayName string            `json:"display_name"`
	Address     map[string]string `json:"address"`
}

type nominatimSearch []nominatimResult

var (
	nominatimSearchCodec = codec.JSONCodec[nominatimSearch]{}
	nominatimArrayCodec  = codec.JSONCodec[[]nominatimResult]{}
	nominatimResultCodec = codec.JSONCodec[nominatimResult]{}
)

// Geocode resolves address to locations via Nominatim search.
func (s *osmGeo) Geocode(ctx context.Context, address string) ([]geo.Location, error) {
	reqURL := s.buildURL("/search", url.Values{
		"q":              {address},
		"format":         {"jsonv2"},
		"addressdetails": {"1"},
		"limit":          {"10"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("geo: osm: create request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(ctx, resp, s.maxBody)
	if err != nil {
		return nil, err
	}
	if statusErr := checkStatus(resp, body); statusErr != nil {
		return nil, statusErr
	}

	results, err := nominatimSearchCodec.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("geo: osm: decode: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("geo: osm: no results for %q: %w", address, geo.ErrNotFound)
	}

	locs := make([]geo.Location, 0, len(results))
	for _, r := range results {
		lat, err := strconv.ParseFloat(r.Lat, 64)
		if err != nil {
			return nil, fmt.Errorf("geo: osm: invalid coordinate %q: %w: %w", r.Lat, err, geo.ErrNotFound)
		}
		lon, err := strconv.ParseFloat(r.Lon, 64)
		if err != nil {
			return nil, fmt.Errorf("geo: osm: invalid coordinate %q: %w: %w", r.Lon, err, geo.ErrNotFound)
		}
		if !geo.ValidCoord(lat, lon) {
			return nil, fmt.Errorf("geo: osm: invalid coordinate (%.6f,%.6f): %w", lat, lon, geo.ErrInvalidCoordinate)
		}
		locs = append(locs, geo.Location{
			Lat:       lat,
			Lng:       lon,
			Formatted: r.DisplayName,
		})
	}
	return locs, nil
}

// ReverseGeocode resolves coordinates to addresses via Nominatim reverse.
func (s *osmGeo) ReverseGeocode(ctx context.Context, lat, lng float64) ([]geo.Address, error) {
	if !geo.ValidCoord(lat, lng) {
		return nil, fmt.Errorf("geo: osm: invalid coordinates: %w", geo.ErrInvalidCoordinate)
	}

	reqURL := s.buildURL("/reverse", url.Values{
		"lat":            {strconv.FormatFloat(lat, 'f', -1, 64)},
		"lon":            {strconv.FormatFloat(lng, 'f', -1, 64)},
		"format":         {"jsonv2"},
		"addressdetails": {"1"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("geo: osm: create request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(ctx, resp, s.maxBody)
	if err != nil {
		return nil, err
	}
	if statusErr := checkStatus(resp, body); statusErr != nil {
		return nil, statusErr
	}

	// Trim space to detect empty body.
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "[]" || trimmed == "{}" {
		return nil, fmt.Errorf("geo: osm: no results for (%.6f,%.6f): %w", lat, lng, geo.ErrNotFound)
	}

	// Try decode as array first.
	arr, decodeErr := nominatimArrayCodec.Decode(body)
	if decodeErr == nil {
		if len(arr) == 0 {
			return nil, fmt.Errorf("geo: osm: no results for (%.6f,%.6f): %w", lat, lng, geo.ErrNotFound)
		}
		// Check if display_name empty indicates no result.
		filtered := make([]geo.Address, 0, len(arr))
		for _, r := range arr {
			if strings.TrimSpace(r.DisplayName) == "" && len(r.Address) == 0 {
				continue
			}
			comp := r.Address
			if comp == nil {
				comp = map[string]string{}
			}
			filtered = append(filtered, geo.Address{
				Formatted:  r.DisplayName,
				Components: comp,
			})
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("geo: osm: no results for (%.6f,%.6f): %w", lat, lng, geo.ErrNotFound)
		}
		return filtered, nil
	}

	// Fallback: single object.
	single, err := nominatimResultCodec.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("geo: osm: decode: %w", err)
	}
	if strings.TrimSpace(single.DisplayName) == "" {
		return nil, fmt.Errorf("geo: osm: no results for (%.6f,%.6f): %w", lat, lng, geo.ErrNotFound)
	}
	comp := single.Address
	if comp == nil {
		comp = map[string]string{}
	}
	return []geo.Address{
		{Formatted: single.DisplayName, Components: comp},
	}, nil
}

// Distance returns great-circle distance in meters using haversine.
// Routing via external service is a future enhancement; current MVP uses haversine only.
func (s *osmGeo) Distance(_ context.Context, from, to geo.Point) (float64, error) {
	if !geo.ValidCoord(from.Lat, from.Lng) || !geo.ValidCoord(to.Lat, to.Lng) {
		return 0, fmt.Errorf("geo: osm: invalid coordinates: %w", geo.ErrInvalidCoordinate)
	}
	return 1000 * haversine(from.Lat, from.Lng, to.Lat, to.Lng), nil
}

// Close releases resources.
func (s *osmGeo) Close() error { return nil }

func (s *osmGeo) do(req *http.Request) (*http.Response, error) {
	if err := s.pace(req.Context()); err != nil {
		return nil, fmt.Errorf("geo: osm: %w", err)
	}

	resp, err := s.client.Do(req)
	// Re-anchor the pacing schedule to the observed request time: every
	// delay between the pace wait and the request hitting the wire
	// (late timer wakeup, scheduler delay under load) would otherwise
	// shrink the following gap below minInterval. Anchoring after the
	// round trip is conservative (gaps grow by the response time) but
	// keeps sequential gaps at >= minInterval under arbitrary load.
	// max() preserves later slots reserved by concurrent callers.
	s.noteSent()
	if err != nil {
		return nil, fmt.Errorf("geo: osm: do: %w", redactURLError(err))
	}
	return resp, nil
}

// pace blocks until minInterval has elapsed since the last request this
// adapter sent, so as not to exceed Nominatim's usage policy (see
// defaultMinRequestInterval). minInterval == 0 (an osmGeo{} literal built
// directly rather than via New) disables pacing.
func (s *osmGeo) pace(ctx context.Context) error {
	if s.minInterval <= 0 {
		return nil
	}

	s.paceMu.Lock()
	now := time.Now()
	wait := s.nextAllowed.Sub(now)
	if wait < 0 {
		wait = 0
	}
	s.nextAllowed = now.Add(wait).Add(s.minInterval)
	s.paceMu.Unlock()

	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// noteSent re-anchors the pacing schedule so the next request waits a
// full minInterval after the request that just completed.
func (s *osmGeo) noteSent() {
	if s.minInterval <= 0 {
		return
	}
	s.paceMu.Lock()
	defer s.paceMu.Unlock()
	if next := time.Now().Add(s.minInterval); next.After(s.nextAllowed) {
		s.nextAllowed = next
	}
}

func redactURLError(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	uStr := ue.URL
	if parsed, pErr := url.Parse(uStr); pErr == nil {
		q := parsed.Query()
		if q.Has("access_token") {
			q.Set("access_token", "REDACTED")
			parsed.RawQuery = q.Encode()
			uStr = parsed.String()
		}
	}
	return &url.Error{Op: ue.Op, URL: uStr, Err: ue.Err}
}

func readLimitedBody(ctx context.Context, resp *http.Response, limit int64) ([]byte, error) {
	data, err := httpclient.ReadLimited(ctx, resp.Body, limit)
	if err != nil {
		if errors.Is(err, httpclient.ErrTooLarge) {
			return nil, fmt.Errorf("geo: osm: %w", geo.ErrTooLarge)
		}
		return nil, fmt.Errorf("geo: osm: read: %w", err)
	}
	return data, nil
}

func checkStatus(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	msg := string(body)
	if len(msg) > 512 {
		msg = msg[:512]
	}
	return fmt.Errorf("geo: osm: status %d: %s", resp.StatusCode, msg)
}

func (s *osmGeo) buildURL(path string, params url.Values) string {
	// endpoint already trimmed of trailing slash
	u := s.endpoint + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	return u
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	a = math.Max(0, math.Min(1, a))
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
