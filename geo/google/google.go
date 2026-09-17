package google

import (
	"context"
	"fmt"
	"net/url"

	gmaps "googlemaps.github.io/maps"

	"github.com/zenta-dev/zever/geo"
	"github.com/zenta-dev/zever/internal/httpclient"
)

type adapter struct {
	client *gmaps.Client
}

var newClient = gmaps.NewClient

// New creates a Google Maps geo adapter from typed options.
func New(opts geo.Options) (geo.Geo, error) {
	if opts.APIKey == "" {
		return nil, fmt.Errorf("geo: google: api_key is required: %w", geo.ErrInvalidOptions)
	}

	if opts.BaseURL != "" {
		if err := validateBaseURL(opts.BaseURL, opts.AllowInsecure); err != nil {
			return nil, err
		}
	}

	var clientOpts []gmaps.ClientOption

	clientOpts = append(clientOpts, gmaps.WithAPIKey(opts.APIKey))
	clientOpts = append(clientOpts, gmaps.WithHTTPClient(httpclient.NewClient(opts.Timeout)))

	if opts.BaseURL != "" {
		clientOpts = append(clientOpts, gmaps.WithBaseURL(opts.BaseURL))
	}

	c, err := newClient(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("geo: google: new client: %w", err)
	}

	return &adapter{client: c}, nil
}

func validateBaseURL(raw string, allowInsecure bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("geo: google: invalid base_url %q: %w: %w", raw, err, geo.ErrInvalidOptions)
	}

	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("geo: google: invalid base_url %q: missing scheme or host: %w", raw, geo.ErrInvalidOptions)
	}

	if u.Scheme == "https" {
		return nil
	}

	if u.Scheme == "http" && allowInsecure {
		return nil
	}

	return fmt.Errorf("geo: google: base_url must use https (got %q): %w", raw, geo.ErrInvalidOptions)
}

func (a *adapter) Geocode(ctx context.Context, address string) ([]geo.Location, error) {
	results, err := a.client.Geocode(ctx, &gmaps.GeocodingRequest{Address: address})
	if err != nil {
		return nil, fmt.Errorf("geo: google: geocode: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("geo: google: no results for %q: %w", address, geo.ErrNotFound)
	}

	var locs []geo.Location
	for _, r := range results {
		locs = append(locs, geo.Location{
			Lat:       r.Geometry.Location.Lat,
			Lng:       r.Geometry.Location.Lng,
			Formatted: r.FormattedAddress,
		})
	}

	return locs, nil
}

func (a *adapter) ReverseGeocode(ctx context.Context, lat, lng float64) ([]geo.Address, error) {
	if !geo.ValidCoord(lat, lng) {
		return nil, fmt.Errorf("geo: google: invalid coordinates, |lat| <= 90 and |lng| <= 180 required: %w", geo.ErrInvalidCoordinate)
	}

	results, err := a.client.ReverseGeocode(ctx, &gmaps.GeocodingRequest{
		LatLng: &gmaps.LatLng{Lat: lat, Lng: lng},
	})
	if err != nil {
		return nil, fmt.Errorf("geo: google: reverse geocode: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("geo: google: no results for (%.4f, %.4f): %w", lat, lng, geo.ErrNotFound)
	}

	var addrs []geo.Address

	for _, r := range results {
		components := make(map[string]string)

		for _, ac := range r.AddressComponents {
			if len(ac.Types) > 0 {
				components[ac.Types[0]] = ac.LongName
			}
		}

		addrs = append(addrs, geo.Address{
			Formatted:  r.FormattedAddress,
			Components: components,
		})
	}

	return addrs, nil
}

func (a *adapter) Distance(ctx context.Context, from geo.Point, to geo.Point) (float64, error) {
	if !geo.ValidCoord(from.Lat, from.Lng) || !geo.ValidCoord(to.Lat, to.Lng) {
		return 0, fmt.Errorf("geo: google: invalid coordinates, |lat| <= 90 and |lng| <= 180 required: %w", geo.ErrInvalidCoordinate)
	}

	resp, err := a.client.DistanceMatrix(ctx, &gmaps.DistanceMatrixRequest{
		Origins:      []string{fmt.Sprintf("%f,%f", from.Lat, from.Lng)},
		Destinations: []string{fmt.Sprintf("%f,%f", to.Lat, to.Lng)},
		Mode:         gmaps.TravelModeDriving,
	})
	if err != nil {
		return 0, fmt.Errorf("geo: google: distance matrix: %w", err)
	}

	if len(resp.Rows) == 0 || len(resp.Rows[0].Elements) == 0 {
		return 0, fmt.Errorf("geo: google: no distance results: %w", geo.ErrNotFound)
	}

	elem := resp.Rows[0].Elements[0]
	if elem.Status != "OK" {
		return 0, fmt.Errorf("geo: google: distance element status: %s: %w", elem.Status, geo.ErrNotFound)
	}

	return float64(elem.Distance.Meters), nil
}

func (a *adapter) Close() error { return nil }
