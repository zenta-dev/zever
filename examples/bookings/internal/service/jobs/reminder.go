package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/notification"
	genapp "github.com/zenta-dev/zever/examples/bookings/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

// SendReminderArgs is the payload of the SendReminder job declared in the
// schema. The job takes no parameters, so the payload is empty.
type SendReminderArgs struct{}

// ReminderWindow bounds how far ahead a booking start counts as due soon.
const ReminderWindow = 7 * 24 * time.Hour

// DueBookings returns confirmed bookings whose start_date falls in
// [now, now+ReminderWindow]. Start dates parse as YYYY-MM-DD first, then
// RFC3339; unparsable rows are skipped.
func DueBookings(ctx context.Context, database db.DB, now time.Time) ([]genapp.Booking, error) {
	rows, err := orm.From(genapp.Bookings).Where(genapp.BookingCols.Status.Eq("confirmed")).All(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("[jobs] reminder: list bookings: %w", err)
	}
	end := now.Add(ReminderWindow)
	var out []genapp.Booking
	for _, b := range rows {
		start, perr := time.Parse("2006-01-02", b.StartDate)
		if perr != nil {
			start, perr = time.Parse(time.RFC3339, b.StartDate)
			if perr != nil {
				continue
			}
		}
		if !start.Before(now) && !start.After(end) {
			out = append(out, *b)
		}
	}
	return out, nil
}

// RunReminder notifies the guest of every due-soon booking and returns how
// many were reminded, so tests can assert on it without reading logs.
func RunReminder(ctx context.Context, deps Deps, now time.Time) (int, error) {
	due, err := DueBookings(ctx, deps.DB, now)
	if err != nil {
		return 0, err
	}
	for _, b := range due {
		guest, ok, gerr := orm.From(genapp.Users).Where(genapp.UserCols.ID.Eq(b.GuestID)).First(ctx, deps.DB)
		if gerr != nil {
			return 0, fmt.Errorf("[jobs] reminder: lookup guest: %w", gerr)
		}
		if !ok {
			return 0, fmt.Errorf("[jobs] reminder: guest %q not found", b.GuestID)
		}
		body := fmt.Sprintf("Reminder: booking %s starts %s.", b.ID, b.StartDate)
		note := notification.NewNotification(guest.Email, notification.ChannelPush, body)
		note.Title = "Upcoming booking"
		note.Data = map[string]string{"booking_id": b.ID}
		if nerr := deps.Notifier.Notify(ctx, &note); nerr != nil {
			return 0, fmt.Errorf("[jobs] reminder: notify guest: %w", nerr)
		}
	}
	deps.Logger.Info().Int("due_bookings", len(due)).Msg("reminder sent")
	return len(due), nil
}

// ReminderHandler returns the job.Register-compatible handler for
// SendReminder.
func ReminderHandler(deps Deps) func(ctx context.Context, args SendReminderArgs) error {
	return func(ctx context.Context, _ SendReminderArgs) error {
		_, err := RunReminder(ctx, deps, time.Now().UTC())
		return err
	}
}
