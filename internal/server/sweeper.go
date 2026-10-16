package server

import (
	"context"
	"time"
)

// blogUploadSweepInterval is how often abandoned post-image uploads are
// reclaimed. An hourly sweep keeps the TTL (24h) honest without hammering R2.
const blogUploadSweepInterval = time.Hour

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
