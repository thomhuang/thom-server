package server

import (
	"context"
	"time"
)

// reservationSweepInterval is how often expired stock holds are released. The
// checkout.session.expired webhook normally frees a hold, but a missed delivery
// would otherwise leave the item reserved until the next checkout request runs
// the lazy sweep.
const reservationSweepInterval = time.Minute

// blogUploadSweepInterval is how often abandoned post-image uploads are
// reclaimed. An hourly sweep keeps the TTL (24h) honest without hammering R2.
const blogUploadSweepInterval = time.Hour

// RunReservationSweeper releases expired stock reservations on a fixed interval
// until ctx is cancelled. It sweeps once before the first tick so a hold that
// expired while the process was down does not linger.
func (app *App) RunReservationSweeper(ctx context.Context) {
	ticker := time.NewTicker(reservationSweepInterval)
	defer ticker.Stop()

	for {
		if err := app.shop.ReleaseExpiredReservations(time.Now().Unix()); err != nil {
			app.infoLog.Printf("reservation sweep failed: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunBlogUploadSweeper deletes pasted post images that were never committed to
// a saved post. It runs once before the first tick so uploads orphaned while
// the process was down are reclaimed.
func (app *App) RunBlogUploadSweeper(ctx context.Context) {
	ticker := time.NewTicker(blogUploadSweepInterval)
	defer ticker.Stop()

	for {
		if err := app.sweepBlogUploads(); err != nil {
			app.infoLog.Printf("blog upload sweep failed: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweepBlogUploads removes expired pending uploads, deleting each object from
// R2 on a best-effort basis before dropping the row.
func (app *App) sweepBlogUploads() error {
	keys, err := app.blog.ExpiredUploads(time.Now())
	if err != nil {
		return err
	}

	for _, key := range keys {
		if err := app.imageStore.Delete(key); err != nil {
			app.infoLog.Printf("failed to delete expired upload %s: %v", key, err)
		}
	}

	return app.blog.DeleteUploads(keys)
}
