package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	genapp "github.com/zenta-dev/zever/examples/bookings/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/mailer"
	"github.com/zenta-dev/zever/notification"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/webhook"
)

// SendConfirmationArgs is the payload of the SendConfirmation job declared in
// the schema.
type SendConfirmationArgs struct {
	BookingID string `json:"booking_id"`
}

// confirmationPayload is the webhook payload fanned out on booking.created.
type confirmationPayload struct {
	ID      string `json:"id"`
	SpaceID string `json:"space_id"`
	GuestID string `json:"guest_id"`
	Status  string `json:"status"`
}

// RunConfirmation loads the booking, sends a mail receipt to the guest,
// pushes a guest notification, and fans out a booking.created webhook event.
// It returns nil only when all three deliveries succeed.
func RunConfirmation(ctx context.Context, deps Deps, bookingID string) error {
	if bookingID == "" {
		return errors.New("[jobs] confirmation: booking id is empty")
	}

	booking, ok, err := orm.From(genapp.Bookings).Where(genapp.BookingCols.ID.Eq(bookingID)).First(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] confirmation: lookup booking: %w", err)
	}
	if !ok {
		return fmt.Errorf("[jobs] confirmation: booking %q not found", bookingID)
	}

	guest, ok, err := orm.From(genapp.Users).Where(genapp.UserCols.ID.Eq(booking.GuestID)).First(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] confirmation: lookup guest: %w", err)
	}
	if !ok {
		return fmt.Errorf("[jobs] confirmation: guest %q not found", booking.GuestID)
	}

	space, ok, err := orm.From(genapp.Spaces).Where(genapp.SpaceCols.ID.Eq(booking.SpaceID)).First(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] confirmation: lookup space: %w", err)
	}
	if !ok {
		return fmt.Errorf("[jobs] confirmation: space %q not found", booking.SpaceID)
	}

	body := fmt.Sprintf("Booking %s for %q (%s to %s) is %s.",
		booking.ID, space.Title, booking.StartDate, booking.EndDate, booking.Status)

	mail := mailer.NewMail(
		mailer.Address{Address: "receipts@example.com", Name: "Bookings"},
		[]mailer.Address{{Address: guest.Email}},
		fmt.Sprintf("Booking %s confirmed", booking.ID),
		body,
	)
	if merr := deps.Mailer.Send(ctx, &mail); merr != nil {
		return fmt.Errorf("[jobs] confirmation: send mail: %w", merr)
	}

	note := notification.NewNotification(guest.Email, notification.ChannelPush, body)
	note.Title = "Booking confirmed"
	note.Data = map[string]string{"booking_id": booking.ID}
	if nerr := deps.Notifier.Notify(ctx, &note); nerr != nil {
		return fmt.Errorf("[jobs] confirmation: notify guest: %w", nerr)
	}

	payload, err := json.Marshal(confirmationPayload{
		ID: booking.ID, SpaceID: booking.SpaceID, GuestID: booking.GuestID, Status: booking.Status,
	})
	if err != nil {
		return fmt.Errorf("[jobs] confirmation: encode payload: %w", err)
	}
	if err := deps.Webhook.Deliver(ctx, EventCreated, payload); err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			deps.Logger.Info().Str("event", EventCreated).Msg("no webhook subscribers")
		} else {
			return fmt.Errorf("[jobs] confirmation: deliver webhook: %w", err)
		}
	}

	deps.Logger.Info().Str("booking_id", booking.ID).Msg("confirmation sent")
	return nil
}

// ConfirmationHandler returns the job.Register-compatible handler for
// SendConfirmation. It closes over deps so the registry holds no hidden state.
func ConfirmationHandler(deps Deps) func(ctx context.Context, args SendConfirmationArgs) error {
	return func(ctx context.Context, args SendConfirmationArgs) error {
		return RunConfirmation(ctx, deps, args.BookingID)
	}
}
