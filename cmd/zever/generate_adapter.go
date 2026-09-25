package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// adapterUsageBody is the long description for `zever generate adapter -h`.
//
// The wording is deliberately blunt about the ceiling: this command writes
// boilerplate, not an integration.
//
//nolint:unused
var adapterUsageBody = `Scaffolds <battery>/<name>/{<name>.go,options.go}: a compiling stub adapter
for an existing zever battery interface, wired up the way every hand-written
adapter in this repo is — one method per interface method plus an exported
New factory taking the battery-level Options.

WHAT THIS DOES NOT DO: it does not and cannot write your adapter's actual
integration logic. Nobody can auto-generate "how to talk to Stripe". Every
generated method body is a "// TODO: implement" that returns the zero value
and a not-implemented error; New returns an empty struct. What you get is
the interface/factory/options boilerplate, spelled correctly, so you can
delete the TODOs one at a time instead of copying another adapter and hoping
you caught every signature.

WIRING (by hand, after filling in the TODOs): zever registers adapters
explicitly, not via init(). Add a Name constant to the battery's Adapter
enum plus its ParseAdapter case in <battery>/adapter.go, register the
constructor in container/services.go's registerAdapters
(_ = <battery>.Register(<battery>.Name, <name>.New), then select it in
zever.yaml (<battery>: {adapter: <name>}).

This is a contributor command: it writes into this repository's own battery
directories, and the generated options.go imports the module-internal
github.com/zenta-dev/zever/internal/opts helpers.

--field pre-declares an Options field, as name:type; it may be repeated, and
may appear before or after the positional arguments. Valid types: ` + adapterOptionTypeList + `.`

//nolint:unused
var adapterUsage = `zever generate adapter <battery> <name> [--field name:type]... [--force]

` + adapterUsageBody + `

Flags:`

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seams keep behavior identical while enabling success/error coverage.
//
//nolint:dupl
var (
	promptSelectForAdapter  = promptSelect
	promptInputForAdapter   = promptInput
	promptConfirmForAdapter = promptConfirm
)

// ErrAdapterUsage is returned when runGenerateAdapter gets the wrong
// positional count.
var ErrAdapterUsage = errors.New("usage: zever generate adapter <battery> <name> [--field name:type] [--force]")

func printAdapterUsage(fs *flag.FlagSet) {
	header := title("zever generate adapter") + dim(" — stub an adapter")
	//nolint:lll
	usage := bold("Usage:") + "  " + cmd("zever generate adapter") + dim(" <battery> <name> [--field name:type] [--force]") + dim("  •  -i/--interactive")

	printBoxedUsage(fs, header, usage, adapterUsageBody, []usageExample{
		{command: "zever generate adapter cache redis --field addr:string --field db:int"},
		{command: "zever generate adapter -i", comment: "  # select battery, name & fields"},
	}, "every method body is a TODO — the integration is yours")
}

// batteryMethod is one method of a battery's interface, captured as source
// text: Params are "name type" pairs and Results are bare type expressions,
// both already qualified with the battery's package name where they name a
// type declared in that package.
type batteryMethod struct {
	Name    string
	Params  []string
	Results []string
}

// batterySpec describes one battery's adapter contract.
type batterySpec struct {
	Package   string
	Interface string
	// Imports lists the non-battery packages the method signatures mention.
	Imports []string
	Methods []batteryMethod
}

// batterySpecs is the battery-name -> interface-shape table.
//
// This is a hand-maintained table rather than go/types introspection of the
// battery packages at generate time, and that is a deliberate trade. Loading
// and type-checking a package with golang.org/x/tools/go/packages would keep
// itself up to date automatically, but it means shelling out to the Go
// toolchain from a CLI that is otherwise dependency-free and instant, and it
// buys accuracy for a set of interfaces that changes about as often as the
// framework's major version. A table entry is also the honest place to record
// the two things introspection gets wrong on its own: an interface name that
// does not match its package (notification.Notifier, permission.Checker,
// ratelimit.Limiter, idempotency.Store, session.Store) and an embedded
// interface from another package (router.Router embeds http.Handler,
// crypto.Crypto embeds Encryptor+Signer, eventbus.EventBus embeds Pusher).
//
// MAINTENANCE COST, stated plainly: changing a battery interface means
// updating its entry here in the same commit, or `zever generate adapter`
// starts emitting stubs that no longer satisfy the interface. The entries
// were mechanically extracted from each <battery> package's interface
// declarations (see the apidump(go/parser) pass in the port notes) and are
// checked in as real source, not derived at runtime.
//
// Absent by design, mirroring the container package: zever has no lock,
// realtime, or secrets batteries, so there are no entries for them here.
// observability is intentionally absent too: its Factory returns the *struct*
// observability.Provider, not an interface, so there is no method set to stub.
var batterySpecs = map[string]batterySpec{
	"ai": {
		Package:   "ai",
		Interface: "AI",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Generate",
				Params:  []string{"ctx context.Context", "model string", "messages []ai.Message", "opts ai.GenerateOptions"},
				Results: []string{"ai.Generation", "error"},
			},
			{
				Name:    "Stream",
				Params:  []string{"ctx context.Context", "model string", "messages []ai.Message", "opts ai.GenerateOptions"},
				Results: []string{"<-chan ai.StreamChunk", "error"},
			},
			{
				Name:    "Embed",
				Params:  []string{"ctx context.Context", "model string", "inputs []string", "opts ai.EmbedOptions"},
				Results: []string{"[][]float32", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"analytics": {
		Package:   "analytics",
		Interface: "Analytics",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Track",
				Params:  []string{"ctx context.Context", "event string", "properties map[string]any"},
				Results: []string{"error"},
			},
			{
				Name:    "Identify",
				Params:  []string{"ctx context.Context", "userID string", "traits map[string]any"},
				Results: []string{"error"},
			},
			{
				Name:    "Group",
				Params:  []string{"ctx context.Context", "userID string", "groupID string", "traits map[string]any"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"auth": {
		Package:   "auth",
		Interface: "Auth",
		Imports:   []string{"context", "time"},
		Methods: []batteryMethod{
			{
				Name:    "Issue",
				Params:  []string{"ctx context.Context", "subject string", "claims map[string]any", "ttl time.Duration"},
				Results: []string{"auth.Token", "error"},
			},
			{
				Name:    "Verify",
				Params:  []string{"ctx context.Context", "token string"},
				Results: []string{"auth.Claims", "error"},
			},
			{
				Name:    "Revoke",
				Params:  []string{"ctx context.Context", "token string"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"billing": {
		Package:   "billing",
		Interface: "Billing",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "CreateCustomer",
				Params:  []string{"ctx context.Context", "name string", "email string"},
				Results: []string{"billing.Customer", "error"},
			},
			{
				Name:    "CreateSubscription",
				Params:  []string{"ctx context.Context", "customerID string", "planID string"},
				Results: []string{"billing.Subscription", "error"},
			},
			{
				Name:    "CancelSubscription",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"error"},
			},
			{
				Name:    "GetInvoice",
				Params:  []string{"ctx context.Context", "customerID string"},
				Results: []string{"billing.Invoice", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"cache": {
		Package:   "cache",
		Interface: "Cache",
		Imports:   []string{"context", "time"},
		Methods: []batteryMethod{
			{
				Name:    "Get",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Set",
				Params:  []string{"ctx context.Context", "key string", "value []byte", "ttl time.Duration"},
				Results: []string{"error"},
			},
			{
				Name:    "SetIfAbsent",
				Params:  []string{"ctx context.Context", "key string", "value []byte", "ttl time.Duration"},
				Results: []string{"bool", "error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"error"},
			},
			{
				Name:    "Increment",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"error"},
			},
			{
				Name:    "Decrement",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"error"},
			},
			{
				Name:    "Exists",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"bool", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
		},
	},
	"crypto": {
		Package:   "crypto",
		Interface: "Crypto",
		Imports:   []string{"context"},
		// Flattened from the embedded Encryptor and Signer interfaces:
		// neither declares Close, so a crypto adapter has no Close method.
		Methods: []batteryMethod{
			{
				Name:    "Encrypt",
				Params:  []string{"ctx context.Context", "plaintext []byte"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Decrypt",
				Params:  []string{"ctx context.Context", "ciphertext []byte"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Sign",
				Params:  []string{"ctx context.Context", "message []byte"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Verify",
				Params:  []string{"ctx context.Context", "message []byte", "signature []byte"},
				Results: []string{"bool", "error"},
			},
			{
				Name:    "Mac",
				Params:  []string{"ctx context.Context", "message []byte"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "VerifyMac",
				Params:  []string{"ctx context.Context", "message []byte", "mac []byte"},
				Results: []string{"bool", "error"},
			},
		},
	},
	"db": {
		Package:   "db",
		Interface: "DB",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Query",
				Params:  []string{"ctx context.Context", "query string", "args ...any"},
				Results: []string{"db.Rows", "error"},
			},
			{
				Name:    "Exec",
				Params:  []string{"ctx context.Context", "query string", "args ...any"},
				Results: []string{"int64", "error"},
			},
			{
				Name:    "Ping",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
			{
				Name:    "Dialect",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"document": {
		Package:   "document",
		Interface: "Document",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Render",
				Params:  []string{"ctx context.Context", "source []byte", "format document.OutputFormat"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"eventbus": {
		Package:   "eventbus",
		Interface: "EventBus",
		Imports:   []string{"context"},
		// Publish/Subscribe/Close/Name are flattened from the embedded
		// Pusher interface.
		Methods: []batteryMethod{
			{
				Name:    "Publish",
				Params:  []string{"ctx context.Context", "topic string", "payload eventbus.Payload", "headers eventbus.Headers"},
				Results: []string{"error"},
			},
			{
				Name:    "Subscribe",
				Params:  []string{"ctx context.Context", "topic string", "handler eventbus.Handler"},
				Results: []string{"func()", "error"},
			},
			{
				Name:    "SubscribeChan",
				Params:  []string{"ctx context.Context", "topic string", "buffer int"},
				Results: []string{"<-chan eventbus.Message", "error"},
			},
			{
				Name:    "Unsubscribe",
				Params:  []string{"topic string", "ch <-chan eventbus.Message"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
			{
				Name:    "Name",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"flag": {
		Package:   "flag",
		Interface: "Flag",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Bool",
				Params:  []string{"ctx context.Context", "key string", "fallback bool"},
				Results: []string{"bool", "error"},
			},
			{
				Name:    "String",
				Params:  []string{"ctx context.Context", "key string", "fallback string"},
				Results: []string{"string", "error"},
			},
			{
				Name:    "Int",
				Params:  []string{"ctx context.Context", "key string", "fallback int"},
				Results: []string{"int", "error"},
			},
			{
				Name:    "JSON",
				Params:  []string{"ctx context.Context", "key string", "out any", "fallback any"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"geo": {
		Package:   "geo",
		Interface: "Geo",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Geocode",
				Params:  []string{"ctx context.Context", "address string"},
				Results: []string{"[]geo.Location", "error"},
			},
			{
				Name:    "ReverseGeocode",
				Params:  []string{"ctx context.Context", "lat float64", "lng float64"},
				Results: []string{"[]geo.Address", "error"},
			},
			{
				Name:    "Distance",
				Params:  []string{"ctx context.Context", "from geo.Point", "to geo.Point"},
				Results: []string{"float64", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"i18n": {
		Package:   "i18n",
		Interface: "I18n",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Translate",
				Params:  []string{"ctx context.Context", "locale string", "key string", "args map[string]string"},
				Results: []string{"string", "error"},
			},
			{
				Name:    "Locales",
				Params:  []string{"ctx context.Context"},
				Results: []string{"[]string", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"idempotency": {
		Package:   "idempotency",
		Interface: "Store",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Begin",
				Params:  []string{"ctx context.Context", "key string", "opts idempotency.BeginOptions"},
				Results: []string{"idempotency.Outcome", "error"},
			},
			{
				Name:    "Complete",
				Params:  []string{"ctx context.Context", "key string", "fingerprint []byte", "result []byte"},
				Results: []string{"error"},
			},
			{
				Name:    "Forget",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"log": {
		Package:   "log",
		Interface: "Logger",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Debug",
				Params:  []string{},
				Results: []string{"log.Event"},
			},
			{
				Name:    "Info",
				Params:  []string{},
				Results: []string{"log.Event"},
			},
			{
				Name:    "Warn",
				Params:  []string{},
				Results: []string{"log.Event"},
			},
			{
				Name:    "Error",
				Params:  []string{},
				Results: []string{"log.Event"},
			},
			{
				Name:    "Fatal",
				Params:  []string{},
				Results: []string{"log.Event"},
			},
			{
				Name:    "With",
				Params:  []string{},
				Results: []string{"log.Context"},
			},
			{
				Name:    "WithContext",
				Params:  []string{"ctx context.Context"},
				Results: []string{"log.Logger"},
			},
			{
				Name:    "Enabled",
				Params:  []string{"level log.Level"},
				Results: []string{"bool"},
			},
			{
				Name:    "Sync",
				Params:  []string{},
				Results: []string{"error"},
			},
			{
				Name:    "Name",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"lock": {
		Package:   "lock",
		Interface: "Locker",
		Imports:   []string{"context", "time"},
		// Flattened from Locker and the returned Lock interface, mirroring
		// the crypto entry: the stub implements both shapes in one struct.
		Methods: []batteryMethod{
			{
				Name:    "TryAcquire",
				Params:  []string{"ctx context.Context", "key string", "ttl time.Duration"},
				Results: []string{"lock.Lock", "bool", "error"},
			},
			{
				Name:    "Acquire",
				Params:  []string{"ctx context.Context", "key string", "ttl time.Duration"},
				Results: []string{"lock.Lock", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
			{
				Name:    "Key",
				Params:  []string{},
				Results: []string{"string"},
			},
			{
				Name:    "Extend",
				Params:  []string{"ctx context.Context", "ttl time.Duration"},
				Results: []string{"error"},
			},
			{
				Name:    "Unlock",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
		},
	},
	"mailer": {
		Package:   "mailer",
		Interface: "Mailer",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Send",
				Params:  []string{"ctx context.Context", "msg *mailer.Mail"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"media": {
		Package:   "media",
		Interface: "Media",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Upload",
				Params:  []string{"ctx context.Context", "path string", "data []byte", "opts media.UploadOptions"},
				Results: []string{"media.Asset", "error"},
			},
			{
				Name:    "Download",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "DownloadRange",
				Params:  []string{"ctx context.Context", "id string", "offset int64", "length int64"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"error"},
			},
			{
				Name:    "Stat",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"media.Info", "error"},
			},
			{
				Name:    "Probe",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"media.Probe", "error"},
			},
			{
				Name:    "Transform",
				Params:  []string{"ctx context.Context", "id string", "ops media.TransformOps"},
				Results: []string{"string", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"notification": {
		Package:   "notification",
		Interface: "Notifier",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Notify",
				Params:  []string{"ctx context.Context", "n *notification.Notification"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"payment": {
		Package:   "payment",
		Interface: "Payment",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "CreatePayment",
				Params:  []string{"ctx context.Context", "req payment.Request"},
				Results: []string{"payment.Result", "error"},
			},
			{
				Name:    "Refund",
				Params:  []string{"ctx context.Context", "id string", "amount int64"},
				Results: []string{"error"},
			},
			{
				Name:    "GetPayment",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"payment.Result", "error"},
			},
			{
				Name:    "WebhookEvent",
				Params:  []string{"ctx context.Context", "raw []byte", "signature string"},
				Results: []string{"payment.Event", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"permission": {
		Package:   "permission",
		Interface: "Checker",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Can",
				Params:  []string{"ctx context.Context", "subject permission.Subject", "action string", "resource permission.Resource"},
				Results: []string{"permission.Decision", "error"},
			},
		},
	},
	"queue": {
		Package:   "queue",
		Interface: "Queue",
		Imports:   []string{"context", "time"},
		Methods: []batteryMethod{
			{
				Name:    "Push",
				Params:  []string{"ctx context.Context", "topic string", "payload queue.Payload", "headers queue.Headers"},
				Results: []string{"error"},
			},
			{
				Name:    "PushDelayed",
				Params:  []string{"ctx context.Context", "topic string", "payload queue.Payload", "headers queue.Headers", "delay time.Duration"},
				Results: []string{"error"},
			},
			{
				Name:    "Pop",
				Params:  []string{"ctx context.Context", "topic string"},
				Results: []string{"queue.Message", "error"},
			},
			{
				Name:    "Ack",
				Params:  []string{"ctx context.Context", "msg queue.Message"},
				Results: []string{"error"},
			},
			{
				Name:    "Nack",
				Params:  []string{"ctx context.Context", "msg queue.Message", "requeue bool"},
				Results: []string{"error"},
			},
			{
				Name:    "Length",
				Params:  []string{"ctx context.Context", "topic string"},
				Results: []string{"int64", "error"},
			},
			{
				Name:    "IsEmpty",
				Params:  []string{"ctx context.Context", "topic string"},
				Results: []string{"bool", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
			{
				Name:    "Name",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"ratelimit": {
		Package:   "ratelimit",
		Interface: "Limiter",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Allow",
				Params:  []string{"ctx context.Context", "key string", "tokens float64"},
				Results: []string{"ratelimit.Decision", "error"},
			},
			{
				Name:    "Reset",
				Params:  []string{"ctx context.Context", "key string"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
			{
				Name:    "Name",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"router": {
		Package:   "router",
		Interface: "Router",
		Imports:   []string{"net/http"},
		// Router embeds http.Handler, so ServeHTTP is part of its method set
		// even though it is not written out in the interface literal.
		Methods: []batteryMethod{
			{
				Name:    "ServeHTTP",
				Params:  []string{"w http.ResponseWriter", "r *http.Request"},
				Results: []string{},
			},
			{
				Name:    "Handle",
				Params:  []string{"method string", "pattern string", "handler http.HandlerFunc"},
				Results: []string{},
			},
			{
				Name:    "Group",
				Params:  []string{"prefix string", "middlewares ...func(http.Handler) http.Handler"},
				Results: []string{"router.Group"},
			},
			{
				Name:    "Use",
				Params:  []string{"middlewares ...func(http.Handler) http.Handler"},
				Results: []string{},
			},
		},
	},
	"scheduler": {
		Package:   "scheduler",
		Interface: "Scheduler",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Schedule",
				Params:  []string{"ctx context.Context", "spec string", "jobName string", "args any"},
				Results: []string{"scheduler.EntryID", "error"},
			},
			{
				Name:    "Remove",
				Params:  []string{"id scheduler.EntryID"},
				Results: []string{"error"},
			},
			{
				Name:    "Entries",
				Params:  []string{},
				Results: []string{"[]scheduler.EntryID"},
			},
			{
				Name:    "Start",
				Params:  []string{},
				Results: []string{"error"},
			},
			{
				Name:    "Stop",
				Params:  []string{},
				Results: []string{"error"},
			},
			{
				Name:    "Name",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"search": {
		Package:   "search",
		Interface: "Search",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Index",
				Params:  []string{"ctx context.Context", "doc search.Document"},
				Results: []string{"error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"error"},
			},
			{
				Name:    "Search",
				Params:  []string{"ctx context.Context", "query string", "opts search.QueryOptions"},
				Results: []string{"search.Result", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"secrets": {
		Package:   "secrets",
		Interface: "Secrets",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Get",
				Params:  []string{"ctx context.Context", "name string"},
				Results: []string{"[]byte", "error"},
			},
			{
				Name:    "Set",
				Params:  []string{"ctx context.Context", "name string", "value []byte"},
				Results: []string{"error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "name string"},
				Results: []string{"error"},
			},
			{
				Name:    "List",
				Params:  []string{"ctx context.Context"},
				Results: []string{"[]string", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
		},
	},
	"session": {
		Package:   "session",
		Interface: "Store",
		Imports:   []string{"context", "time"},
		Methods: []batteryMethod{
			{
				Name:    "Create",
				Params:  []string{"ctx context.Context", "ttl time.Duration"},
				Results: []string{"session.Session", "error"},
			},
			{
				Name:    "Get",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"session.Session", "error"},
			},
			{
				Name:    "Save",
				Params:  []string{"ctx context.Context", "sess session.Session"},
				Results: []string{"error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"storage": {
		Package:   "storage",
		Interface: "Storage",
		Imports:   []string{"context", "time"},
		Methods: []batteryMethod{
			{
				Name:    "PresignUpload",
				Params:  []string{"ctx context.Context", "bucket string", "key string", "contentType string", "ttl time.Duration"},
				Results: []string{"storage.PresignedURL", "error"},
			},
			{
				Name:    "PresignDownload",
				Params:  []string{"ctx context.Context", "bucket string", "key string", "ttl time.Duration"},
				Results: []string{"storage.PresignedURL", "error"},
			},
			{
				Name:    "Exists",
				Params:  []string{"ctx context.Context", "bucket string", "key string"},
				Results: []string{"bool", "error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "bucket string", "key string"},
				Results: []string{"error"},
			},
			{
				Name:    "Move",
				Params:  []string{"ctx context.Context", "srcBucket string", "srcKey string", "dstBucket string", "dstKey string"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{"ctx context.Context"},
				Results: []string{"error"},
			},
			{
				Name:    "Name",
				Params:  []string{},
				Results: []string{"string"},
			},
		},
	},
	"tenant": {
		Package:   "tenant",
		Interface: "Tenant",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Resolve",
				Params:  []string{"ctx context.Context", "meta map[string]string"},
				Results: []string{"string", "error"},
			},
			{
				Name:    "Scoped",
				Params:  []string{"ctx context.Context", "tenantID string"},
				Results: []string{"context.Context", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"vectorstore": {
		Package:   "vectorstore",
		Interface: "VectorStore",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Upsert",
				Params:  []string{"ctx context.Context", "vec vectorstore.Vector"},
				Results: []string{"error"},
			},
			{
				Name:    "Delete",
				Params:  []string{"ctx context.Context", "id string"},
				Results: []string{"error"},
			},
			{
				Name:    "Query",
				Params:  []string{"ctx context.Context", "embedding []float32", "topK int"},
				Results: []string{"[]vectorstore.ScoreMatch", "error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"webhook": {
		Package:   "webhook",
		Interface: "Webhook",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Register",
				Params:  []string{"ctx context.Context", "event string", "target string", "secret string"},
				Results: []string{"error"},
			},
			{
				Name:    "Unregister",
				Params:  []string{"ctx context.Context", "event string", "target string"},
				Results: []string{"error"},
			},
			{
				Name:    "Deliver",
				Params:  []string{"ctx context.Context", "event string", "payload []byte"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
	"workflow": {
		Package:   "workflow",
		Interface: "Workflow",
		Imports:   []string{"context"},
		Methods: []batteryMethod{
			{
				Name:    "Start",
				Params:  []string{"ctx context.Context", "name string", "input any", "workflowID string"},
				Results: []string{"workflow.RunID", "error"},
			},
			{
				Name:    "Signal",
				Params:  []string{"ctx context.Context", "runID workflow.RunID", "name string", "value any"},
				Results: []string{"error"},
			},
			{
				Name:    "Query",
				Params:  []string{"ctx context.Context", "runID workflow.RunID", "name string", "out any"},
				Results: []string{"error"},
			},
			{
				Name:    "Cancel",
				Params:  []string{"ctx context.Context", "runID workflow.RunID"},
				Results: []string{"error"},
			},
			{
				Name:    "Close",
				Params:  []string{},
				Results: []string{"error"},
			},
		},
	},
}

// --- Options field types ---

// adapterOptionType maps a --field type name onto the Go type used in the
// generated Options struct and the internal/opts helper that parses it.
type adapterOptionType struct {
	GoType string
	// Parse is a format string taking the option key, producing the
	// right-hand side of the ParseOptions assignment.
	Parse string
	// Import is the extra stdlib import the Go type needs, if any.
	Import string
}

var adapterOptionTypes = map[string]adapterOptionType{
	"string":   {GoType: "string", Parse: `opts.String(m, %q, "")`},
	"bool":     {GoType: "bool", Parse: `opts.Bool(m, %q, false)`},
	"int":      {GoType: "int", Parse: `opts.Int(m, %q, 0)`},
	"int64":    {GoType: "int64", Parse: `opts.Int64(m, %q, 0)`},
	"float64":  {GoType: "float64", Parse: `opts.Float64(m, %q, 0)`},
	"duration": {GoType: "time.Duration", Parse: `opts.Duration(m, %q, 0)`, Import: "time"},
	"strings":  {GoType: "[]string", Parse: `opts.StringSlice(m, %q)`},
	"map":      {GoType: "map[string]any", Parse: `opts.Map(m, %q)`},
}

// adapterOptionTypeList is the sorted, comma-separated set of --field types,
// used in both the usage text and the error message for a bad --field.
var adapterOptionTypeList = strings.Join(sortedKeys(adapterOptionTypes), ", ")

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// optionSpecs collects a repeatable --field flag for `generate adapter`.
//
// It is deliberately separate from generate entity's fieldSpecs: that flag
// takes .zen scalar types for a schema, this one takes Go/opts types for a
// struct field, and the two vocabularies have no overlap worth sharing.
type optionSpecs []optionSpec

type optionSpec struct {
	Key  string // the config key, as written: snake_case
	Type string // a key of adapterOptionTypes
}

func (o *optionSpecs) String() string {
	parts := make([]string, 0, len(*o))
	for _, spec := range *o {
		parts = append(parts, spec.Key+":"+spec.Type)
	}

	return strings.Join(parts, ",")
}

func (o *optionSpecs) Set(value string) error {
	key, typ, ok := strings.Cut(value, ":")

	key = strings.TrimSpace(key)
	typ = strings.TrimSpace(typ)

	if !ok || key == "" || typ == "" {
		return fmt.Errorf("--field %q: want name:type (for example api_key:string)", value)
	}

	if !isIdent(key) {
		return fmt.Errorf("--field %q: %q is not a valid option name", value, key)
	}

	if _, known := adapterOptionTypes[typ]; !known {
		return fmt.Errorf("--field %q: unknown type %q (want one of %s)", value, typ, adapterOptionTypeList)
	}

	for _, existing := range *o {
		if existing.Key == key {
			return fmt.Errorf("--field %q: option %q declared twice", value, key)
		}
	}

	*o = append(*o, optionSpec{Key: key, Type: typ})

	return nil
}

// --- the command ---

// AdapterOption is one --field declaration for GenerateAdapter: a
// snake_case config key and an adapterOptionTypes key. It mirrors optionSpec
// in exported form so callers can construct adapter input without
// touching flag parsing.
type AdapterOption struct {
	Key  string
	Type string
}

// GenerateAdapterConfig is the pure input to GenerateAdapter: the battery,
// the adapter (Go package) name, its option fields, and the force flag.
type GenerateAdapterConfig struct {
	Battery string
	Name    string
	Fields  []AdapterOption
	Force   bool
	Stdout  io.Writer
	Stderr  io.Writer
}

// GenerateAdapterResult names everything GenerateAdapter wrote.
type GenerateAdapterResult struct {
	Dir         string
	AdapterPath string
	OptionsPath string
}

// GenerateAdapter renders the adapter stub and options files from resolved
// inputs and writes them to disk. Both files render before either is
// written, so a collision on options.go cannot leave a half-scaffolded
// package behind. It performs no flag parsing and no prompting.
//
//nolint:unparam // result kept for callers/tests; user output goes to cfg Stdout/Stderr writers.
func GenerateAdapter(cfg GenerateAdapterConfig) (GenerateAdapterResult, error) {
	const tag = "zever generate adapter"

	var res GenerateAdapterResult

	spec, known := batterySpecs[cfg.Battery]
	if !known {
		return res, fmt.Errorf("%s: unknown battery %q (want one of: %s)", tag, cfg.Battery, strings.Join(sortedKeys(batterySpecs), ", "))
	}

	if isTraversalName(cfg.Name) {
		return res, fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, cfg.Name)
	}

	if !isPackageName(cfg.Name) {
		return res, fmt.Errorf("%s: adapter name %q must be a Go package name (lowercase letters and digits, starting with a letter)", tag, cfg.Name)
	}

	fields := make(optionSpecs, 0, len(cfg.Fields))
	for _, f := range cfg.Fields {
		if err := fields.Set(f.Key + ":" + f.Type); err != nil {
			return res, fmt.Errorf("%s: %w", tag, err)
		}
	}

	// Both path components are re-derived from validated values — the battery
	// directory from the table entry, not the argument, and the adapter name
	// from a fresh [a-z][a-z0-9]* string — so no traversal can reach the
	// filesystem calls below.
	dir := filepath.Join(spec.Package, sanitizedPackageName(cfg.Name))

	adapterSrc, err := renderAdapterFile(tag, spec, cfg.Name)
	if err != nil {
		return res, err
	}

	optionsSrc, err := renderAdapterOptionsFile(tag, cfg.Battery, cfg.Name, fields)
	if err != nil {
		return res, err
	}

	adapterPath := filepath.Join(dir, cfg.Name+".go")
	optionsPath := filepath.Join(dir, "options.go")

	res.Dir = dir
	res.AdapterPath = adapterPath
	res.OptionsPath = optionsPath

	// Both files are checked before either is written, so a collision on
	// options.go cannot leave a half-scaffolded package on disk.
	if !cfg.Force {
		for _, path := range []string{adapterPath, optionsPath} {
			if _, statErr := os.Stat(path); statErr == nil {
				return res, fmt.Errorf("%s: %q already exists (pass --force to overwrite)", tag, path)
			}
		}
	}

	if err := writeScaffold(tag, adapterPath, adapterSrc, cfg.Force); err != nil {
		return res, err
	}

	if err := writeScaffold(tag, optionsPath, optionsSrc, cfg.Force); err != nil {
		return res, err
	}

	if stdout := cfg.Stdout; stdout != nil {
		_, _ = fmt.Fprintf(stdout, "%s %s %s %s %s\n",
			//nolint:lll
			successMark(), success("scaffolded"), bold(cfg.Battery), cyan(fmt.Sprintf("adapter %q", cfg.Name)), dim("at")+cyan(" "+dir)+dim(" (every method body is a TODO)"))
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint(hintFor("generate adapter")))
	}

	return res, nil
}

//nolint:gocyclo
func runGenerateAdapter(args []string) error {
	const tag = "zever generate adapter"

	args = peelInteractive(args)

	var fields optionSpecs

	fs := flag.NewFlagSet("generate adapter", flag.ContinueOnError)
	fs.Var(&fields, "field", "an Options field to declare, as name:type; repeatable")
	force := fs.Bool("force", false, "overwrite the adapter files if they already exist")
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	fs.Usage = func() {
		printAdapterUsage(fs)
	}

	positional, rest := splitPositionals(args, 2)

	if err := fs.Parse(rest); err != nil {
		return err
	}

	positional = append(positional, fs.Args()...)

	if len(positional) < 2 && isInteractiveTerminal() {
		if len(positional) < 1 {
			batteries := sortedKeys(batterySpecs)

			sel, err := promptSelectForAdapter("Battery", batteries)
			if err != nil {
				return err
			}

			positional = append(positional, sel)
		}

		if len(positional) < 2 {
			val, err := promptInputForAdapter("Adapter name (go package)", "", func(s string) error {
				if !isPackageName(s) {
					return errors.New("lowercase letters/digits, starting with letter")
				}

				return nil
			})
			if err != nil {
				return err
			}

			positional = append(positional, val)
		}
		// Interactive fields: loop prompting for name:type
		if len(fields) == 0 {
			for {
				add, err := promptConfirmForAdapter("Add an Options field?", "Yes")
				if err != nil {
					return err
				}

				if !add {
					break
				}

				key, err := promptInputForAdapter("Field name (snake_case)", "", func(s string) error {
					if !isIdent(s) {
						return errors.New("must be identifier")
					}

					return nil
				})
				if err != nil {
					return err
				}

				typ, err := promptSelectForAdapter("Field type", sortedKeys(adapterOptionTypes))
				if err != nil {
					return err
				}

				if err := fields.Set(key + ":" + typ); err != nil {
					_, _ = fmt.Fprintln(os.Stderr, failure(err.Error()))
					continue
				}
			}
		}
	}

	if len(positional) != 2 {
		return fmt.Errorf("%s: %w", tag, ErrAdapterUsage)
	}

	battery, name := positional[0], positional[1]

	if _, known := batterySpecs[battery]; !known {
		if shouldShowHint() {
			if s := closest(battery, sortedKeys(batterySpecs)); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return fmt.Errorf("%s: unknown battery %q (want one of: %s)", tag, battery, strings.Join(sortedKeys(batterySpecs), ", "))
	}

	exported := make([]AdapterOption, 0, len(fields))
	for _, f := range fields {
		exported = append(exported, AdapterOption(f))
	}

	_, err := GenerateAdapter(GenerateAdapterConfig{
		Battery: battery,
		Name:    name,
		Fields:  exported,
		Force:   *force,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})

	return err
}

// sanitizedPackageName rebuilds an already-validated package name character by
// character. isPackageName has proven the name holds nothing but [a-z0-9], so
// this is a no-op at runtime; it exists so the value that reaches filepath.Join
// is constructed here rather than carried in from argv.
func sanitizedPackageName(name string) string {
	var b strings.Builder

	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// isPackageName reports whether s is usable as a Go package name.
func isPackageName(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		default:
			return false
		}
	}

	return true
}

// --- rendering ---

type adapterMethodData struct {
	Signature string
	Body      string
}

type adapterFileData struct {
	Battery   string
	Name      string
	Interface string
	// AdapterConst is the suggested Adapter enum constant for the wiring
	// comment (e.g. "Memcached"). It names nothing in code: adding the real
	// constant to the battery package is a manual wiring step.
	AdapterConst string
	// StdImports and LocalImports are kept apart so the template can emit
	// them as the two groups goimports (local prefix
	// github.com/zenta-dev/zever) would produce.
	StdImports   []string
	LocalImports []string
	Methods      []adapterMethodData
	NeedsErrors  bool
}

const adapterFileTemplate = `// Package {{.Name}} provides a {{.Name}} adapter for the {{.Battery}} battery.
//
// SCAFFOLD: every method below is a stub, and this file is meant to be
// edited. ` + "`zever generate adapter {{.Battery}} {{.Name}}`" + ` wrote the
// interface/factory boilerplate and nothing else — it cannot know how to
// talk to whatever {{.Name}} is. Replace each "TODO: implement" body (and
// New) with the real integration; ` + "`make lint`" + ` will keep reporting the
// TODOs until you do. Delete this paragraph when the adapter is real.
//
// WIRING: zever registers adapters explicitly in
// container/services.go's registerAdapters, not via init(). After filling
// in the TODOs, add a {{.AdapterConst}} constant to the {{.Battery}} Adapter enum
// (plus its ParseAdapter case), register the constructor
// (_ = {{.Battery}}.Register({{.Battery}}.{{.AdapterConst}}, New)), and
// select it in zever.yaml. Until then this package builds but is
// unreachable.
package {{.Name}}

import (
{{range .StdImports}}	"{{.}}"
{{end}}
{{range .LocalImports}}	"{{.}}"
{{end}})
{{if .NeedsErrors}}
// errNotImplemented is returned by every stub method that has not been
// implemented yet. Delete it once none are left.
var errNotImplemented = errors.New("[{{.Battery}}] {{.Name}}: not implemented")
{{end}}
type adapter struct{}
{{range .Methods}}
func (a *adapter) {{.Signature}} {
{{.Body}}}
{{end}}
// New builds the adapter from the battery-level options.
//
// TODO: implement. Map {{.Battery}}.Options onto the connection or client
// the adapter needs, and fail here rather than at first use if it cannot be
// established. Register the result by hand (see the package comment): there
// is no init() here on purpose.
func New(_ {{.Battery}}.Options) ({{.Battery}}.{{.Interface}}, error) {
	return &adapter{}, nil
}
`

func renderAdapterFile(tag string, spec batterySpec, name string) ([]byte, error) {
	data := adapterFileData{
		Battery:      spec.Package,
		Name:         name,
		Interface:    spec.Interface,
		AdapterConst: goIdent(name),
		LocalImports: []string{"github.com/zenta-dev/zever/" + spec.Package},
	}

	stdImports := append([]string(nil), spec.Imports...)

	for _, m := range spec.Methods {
		body, usesErrors := adapterMethodBody(m)
		if usesErrors {
			data.NeedsErrors = true
		}

		data.Methods = append(data.Methods, adapterMethodData{
			Signature: fmt.Sprintf("%s(%s) %s", m.Name, strings.Join(m.Params, ", "), resultList(m.Results)),
			Body:      body,
		})
	}

	if data.NeedsErrors {
		stdImports = append(stdImports, "errors")
	}

	sort.Strings(stdImports)

	data.StdImports = stdImports

	return renderGoFile(tag, "adapter", adapterFileTemplate, data)
}

// resultList renders a method's results as a Go result list: "" for none,
// a bare type for one, a parenthesized list for more.
func resultList(results []string) string {
	switch len(results) {
	case 0:
		return ""
	case 1:
		return results[0] + " "
	default:
		return "(" + strings.Join(results, ", ") + ") "
	}
}

// adapterMethodBody renders a stub body: a TODO, a zero value per non-error
// result, and errNotImplemented for the error result. It reports whether the
// body references errNotImplemented.
//
// Zero values are declared as `var zero T` rather than written out as
// literals, because T can be any type at all — a struct, an interface, a
// channel, a func — and `var` is the one form that is correct for all of them.
func adapterMethodBody(m batteryMethod) (body string, usesErrors bool) {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "\t// TODO: implement %s.\n", m.Name)

	if len(m.Results) == 0 {
		return b.String(), false
	}

	zeros := 0

	for _, r := range m.Results {
		if r != "error" {
			zeros++
		}
	}

	names := make([]string, 0, len(m.Results))
	next := 0

	for _, r := range m.Results {
		if r == "error" {
			usesErrors = true

			names = append(names, "errNotImplemented")

			continue
		}

		next++

		zeroName := "zero"
		if zeros > 1 {
			zeroName = fmt.Sprintf("zero%d", next)
		}

		_, _ = fmt.Fprintf(&b, "\tvar %s %s\n", zeroName, r)
		names = append(names, zeroName)
	}

	if zeros > 0 {
		b.WriteString("\n")
	}

	_, _ = fmt.Fprintf(&b, "\treturn %s\n", strings.Join(names, ", "))

	return b.String(), usesErrors
}

type adapterOptionsData struct {
	Battery      string
	Name         string
	Fields       []adapterOptionsField
	StdImports   []string
	LocalImports []string
	HasFields    bool
	ParseParam   string
}

type adapterOptionsField struct {
	GoName  string
	GoType  string
	Key     string
	Comment string
	Assign  string
}

const adapterOptionsTemplate = `package {{.Name}}
{{if .LocalImports}}
import (
{{range .StdImports}}	"{{.}}"
{{end}}
{{range .LocalImports}}	"{{.}}"
{{end}})
{{end}}
// Options configures the {{.Name}} {{.Battery}} adapter.{{if not .HasFields}}
//
// SCAFFOLD: no fields were declared. Add the ones this adapter needs (and a
// matching line in ParseOptions), or re-run the generator with --field.{{end}}
{{if .HasFields}}type Options struct {
{{- range .Fields}}
	// {{.Comment}}
	{{.GoName}} {{.GoType}} ` + "`json:\"{{.Key}}\" toml:\"{{.Key}}\" yaml:\"{{.Key}}\"`" + `
{{- end}}
}

{{else}}type Options struct{}
{{end}}
// ParseOptions extracts typed options from a raw configuration map.
//
// TODO: add the required-field checks this adapter needs, in the style of the
// other adapters: an empty value that the adapter cannot run without should
// fail here with a "[{{.Battery}}] {{.Name}}: <field> is required" error, not
// at first use.
func ParseOptions({{.ParseParam}} map[string]any) (Options, error) {
{{- if .HasFields}}
	var o Options
{{range .Fields}}
	o.{{.GoName}} = {{.Assign}}
{{- end}}

	return o, nil
{{- else}}
	return Options{}, nil
{{- end}}
}
`

func renderAdapterOptionsFile(tag, battery, name string, fields optionSpecs) ([]byte, error) {
	data := adapterOptionsData{
		Battery:    battery,
		Name:       name,
		HasFields:  len(fields) > 0,
		ParseParam: "_",
	}

	if data.HasFields {
		data.ParseParam = "m"
		data.LocalImports = append(data.LocalImports, "github.com/zenta-dev/zever/internal/opts")
	}

	extra := map[string]bool{}

	for _, f := range fields {
		typ := adapterOptionTypes[f.Type]

		if typ.Import != "" {
			extra[typ.Import] = true
		}

		data.Fields = append(data.Fields, adapterOptionsField{
			GoName:  goFieldName(f.Key),
			GoType:  typ.GoType,
			Key:     f.Key,
			Comment: fmt.Sprintf("%s is the %q option. TODO: document it.", goFieldName(f.Key), f.Key),
			Assign:  fmt.Sprintf(typ.Parse, f.Key),
		})
	}

	// gofmt sorts a contiguous import block alphabetically, which would put
	// the module-local opts import above "time"; goimports' local-prefix
	// grouping wants them in separate blocks, so keep the two lists apart.
	data.StdImports = sortedKeys(extra)

	return renderGoFile(tag, "adapter options", adapterOptionsTemplate, data)
}

// goInitialisms are the words Go style spells in all caps, so that
// --field api_key:string yields APIKey rather than ApiKey.
var goInitialisms = map[string]string{
	"api": "API", "db": "DB", "dsn": "DSN", "html": "HTML", "http": "HTTP",
	"https": "HTTPS", "id": "ID", "json": "JSON", "sql": "SQL", "ssl": "SSL",
	"tls": "TLS", "ttl": "TTL", "uri": "URI", "url": "URL", "uuid": "UUID",
	"xml": "XML",
}

// goFieldName converts a snake_case config key to an exported Go field name.
func goFieldName(key string) string {
	parts := strings.Split(key, "_")

	var b strings.Builder

	for _, part := range parts {
		if part == "" {
			continue
		}

		lower := strings.ToLower(part)
		if initialism, ok := goInitialisms[lower]; ok {
			b.WriteString(initialism)

			continue
		}

		b.WriteString(strings.ToUpper(lower[:1]))
		b.WriteString(lower[1:])
	}

	return b.String()
}
