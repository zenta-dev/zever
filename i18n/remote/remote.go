package remote

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/internal/httpclient"
	"github.com/zenta-dev/zever/internal/lrucache"
)

var (
	requestCodec       = codec.JSONCodec[any]{}
	translateRespCodec = codec.JSONCodec[translateResponse]{}
	localesRespCodec   = codec.JSONCodec[localesResponse]{}
)

const (
	maxCacheEntries = 1024
	posTTL          = 5 * time.Minute
	negTTL          = 30 * time.Second
	maxRespBody     = 1 << 20
	maxErrBody      = 4096
	maxInFlightDef  = 1000
)

var _ i18n.I18n = (*adapter)(nil)

// cacheValue is the payload stored in a.cache, keyed by the full cache key.
// Expiry is no longer tracked here (expiresAt) since it's owned by
// lrucache.TTLCache, which is given the entry's ttl explicitly on each store
// via PutTTL (posTTL for a positive hit, negTTL for a negative "key not
// found" miss).
type cacheValue struct {
	val  string
	miss bool // negative miss: key absent server-side
}

type call struct {
	done   chan struct{}
	val    string
	err    error
	cancel context.CancelFunc
}

type adapter struct {
	mu        sync.Mutex
	closed    bool
	quit      chan struct{}
	wg        sync.WaitGroup
	cache     *lrucache.TTLCache[string, cacheValue]
	flights   map[string]*call
	order     *list.List // FIFO of in-flight keys; front = newest
	orderElem map[string]*list.Element
	maxFlight int
	endpoint  string
	apiKey    string
	client    *http.Client

	locales    []string
	localesExp time.Time
}

// New builds a remote i18n.I18n from opts.
// It validates opts first, requires a non-empty https endpoint
// (http only with AllowInsecure), and resolves Timeout locally:
// opts.Remote.Timeout <= 0 means i18n.DefaultTimeout. (Core timeout()
// is unexported so the adapter cannot call it; this mirrors its logic.)
// MaxInFlight <= 0 means 1000.
func New(opts i18n.Options) (i18n.I18n, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("remote: %w", err)
	}
	endpoint := strings.TrimRight(opts.Remote.Endpoint, "/")
	if endpoint == "" {
		return nil, &i18n.InvalidOptionsError{Reason: "endpoint required"}
	}
	timeout := opts.Remote.Timeout
	if timeout <= 0 {
		timeout = i18n.DefaultTimeout
	}
	maxFlight := opts.Remote.MaxInFlight
	if maxFlight <= 0 {
		maxFlight = maxInFlightDef
	}
	return &adapter{
		quit:      make(chan struct{}),
		cache:     lrucache.NewTTL[string, cacheValue](maxCacheEntries, posTTL),
		flights:   make(map[string]*call),
		order:     list.New(),
		orderElem: make(map[string]*list.Element),
		maxFlight: maxFlight,
		endpoint:  endpoint,
		apiKey:    opts.Remote.APIKey,
		client:    httpclient.NewClient(timeout),
	}, nil
}

// cacheKey is locale\x00key\x00 + explicitly sorted-args JSON.
// (encoding/json sorts map keys by spec; sorted here explicitly anyway.)
func cacheKey(locale, key string, args map[string]string) string {
	var b strings.Builder
	b.WriteString(locale)
	b.WriteByte(0)
	b.WriteString(key)
	b.WriteByte(0)
	if args == nil {
		b.WriteString("null")
		return b.String()
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k) // string keys always marshal (infallible)
		b.Write(kb)
		b.WriteByte(':')
		vb, _ := json.Marshal(args[k]) // string values always marshal (infallible)
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.String()
}

// lookup returns the cached value for key, moving it to front of recency
// order (and lazily purging it if past its TTL) via the underlying
// TTLCache. Call with mu held, so the cache lookup and the caller's
// subsequent flights check happen atomically with respect to a concurrent
// store/Translate.
func (a *adapter) lookup(key string) (cacheValue, bool) {
	return a.cache.Get(key)
}

// store inserts v for key with the given ttl (posTTL for a positive hit,
// negTTL for a negative "key not found" miss), evicting the LRU tail past
// maxCacheEntries via the underlying TTLCache. Call with mu held.
func (a *adapter) store(key string, v cacheValue, ttl time.Duration) {
	a.cache.PutTTL(key, v, ttl)
}

func (a *adapter) Translate(ctx context.Context, locale, key string, args map[string]string) (string, error) {
	if locale == "" {
		return "", &i18n.LocaleNotFoundError{Locale: locale}
	}
	ck := cacheKey(locale, key, args)

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return "", fmt.Errorf("remote: translate: %w", i18n.ErrClosed)
	}
	if e, ok := a.lookup(ck); ok {
		a.mu.Unlock()
		if e.miss {
			return "", &i18n.KeyNotFoundError{Locale: locale, Key: key}
		}
		return e.val, nil
	}
	if f, ok := a.flights[ck]; ok {
		a.mu.Unlock()
		select {
		case <-f.done:
			if f.err != nil {
				return "", f.err
			}
			return f.val, nil
		case <-ctx.Done():
			return "", fmt.Errorf("remote: translate: %w", ctx.Err())
		case <-a.quit:
			return "", fmt.Errorf("remote: translate: %w", i18n.ErrClosed)
		}
	}
	// Caller still holds a.mu from the first closed-check; Close needs a.mu,
	// so closed cannot flip here. Proceed to lead the flight.
	fctx, cancel := context.WithCancel(context.Background())
	f := &call{done: make(chan struct{}), cancel: cancel}
	// order and flights move in lockstep (see insert/cleanup below) and
	// maxFlight >= 1 via New, so back is never nil and holds only keys.
	for len(a.flights) >= a.maxFlight {
		back := a.order.Back()
		victim, ok := back.Value.(string) // order stores only flight keys
		if !ok {
			break
		}
		a.order.Remove(back)
		delete(a.orderElem, victim)
		delete(a.flights, victim)
	}
	a.flights[ck] = f
	a.orderElem[ck] = a.order.PushFront(ck)
	a.wg.Add(1)
	a.mu.Unlock()

	val, err := a.fetch(fctx, locale, key, args)
	cancel()

	a.mu.Lock()
	delete(a.flights, ck)
	if el, ok := a.orderElem[ck]; ok {
		a.order.Remove(el)
		delete(a.orderElem, ck)
	}
	if err == nil {
		a.store(ck, cacheValue{val: val}, posTTL)
	} else if errors.Is(err, i18n.ErrKeyNotFound) {
		a.store(ck, cacheValue{miss: true}, negTTL)
	}
	a.wg.Done()
	a.mu.Unlock()

	f.val, f.err = val, err
	close(f.done)
	return val, err
}

type translateRequest struct {
	Locale string            `json:"locale"`
	Key    string            `json:"key"`
	Args   map[string]string `json:"args"`
}

type translateResponse struct {
	Translations map[string]string `json:"translations"`
}

func (a *adapter) fetch(ctx context.Context, locale, key string, args map[string]string) (string, error) {
	data, err := a.do(ctx, "translate", http.MethodPost, a.endpoint+"/translate", translateRequest{
		Locale: locale, Key: key, Args: args,
	})
	if err != nil {
		return "", err
	}
	resp, err := translateRespCodec.Decode(data)
	if err != nil {
		return "", fmt.Errorf("remote: translate: decode: %w", err)
	}
	if v, ok := resp.Translations[key]; ok {
		return v, nil
	}
	return "", &i18n.KeyNotFoundError{Locale: locale, Key: key}
}

type localesResponse struct {
	Locales []string `json:"locales"`
}

func (a *adapter) Locales(ctx context.Context) ([]string, error) {
	now := time.Now()
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, fmt.Errorf("remote: locales: %w", i18n.ErrClosed)
	}
	if a.locales != nil && a.localesExp.After(now) {
		out := append([]string(nil), a.locales...)
		a.mu.Unlock()
		return out, nil
	}
	a.mu.Unlock()

	data, err := a.do(ctx, "locales", http.MethodGet, a.endpoint+"/locales", nil)
	if err != nil {
		return nil, err
	}
	resp, err := localesRespCodec.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("remote: locales: decode: %w", err)
	}
	out := append([]string(nil), resp.Locales...)

	a.mu.Lock()
	if !a.closed {
		a.locales = append([]string(nil), resp.Locales...)
		a.localesExp = time.Now().Add(posTTL)
	}
	a.mu.Unlock()
	return out, nil
}

func (a *adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	close(a.quit)
	for _, f := range a.flights {
		f.cancel()
	}
	a.mu.Unlock()

	a.wg.Wait()

	a.mu.Lock()
	a.cache = nil
	a.flights = nil
	a.order = nil
	a.orderElem = nil
	a.locales = nil
	a.mu.Unlock()
	return nil
}

func (a *adapter) do(ctx context.Context, op, method, url string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		payload, err := requestCodec.Encode(body)
		if err != nil {
			return nil, fmt.Errorf("remote: encode: %w", err)
		}
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, fmt.Errorf("remote: request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote: %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		eb, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
		return nil, fmt.Errorf("%w: %s: %d: %s", i18n.ErrRemoteError, op, resp.StatusCode, strings.TrimSpace(string(eb)))
	}
	data, err := httpclient.ReadLimited(resp.Body, maxRespBody)
	if err != nil {
		if errors.Is(err, httpclient.ErrTooLarge) {
			return nil, fmt.Errorf("%w: %s: response body exceeds %d bytes", i18n.ErrRemoteError, op, maxRespBody)
		}
		return nil, fmt.Errorf("remote: %s: %w", op, err)
	}
	return data, nil
}
