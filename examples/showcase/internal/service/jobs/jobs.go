// Package jobs implements the background jobs declared in
// schema/shop/shop.zen: SendConfirmation, ProcessOrder, ReindexSearch and
// GenerateDailyReport.
package jobs

import (
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/mailer"
	"github.com/zenta-dev/zever/notification"
)

// Deps carries the resolved services job handlers need. It keeps the global
// job registry free of hidden state: the worker builds one Deps and closes
// over it when registering handlers.
type Deps struct {
	DB       db.DB
	Mailer   mailer.Mailer
	Notifier notification.Notifier
	Logger   log.Logger
}
