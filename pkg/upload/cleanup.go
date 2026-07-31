package upload

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type CleanupStats struct {
	Scanned int
	Deleted int
	Skipped int
	Failed  int
}

func Cleanup(ctx context.Context, store FileStore, referenced map[string]struct{}, now time.Time, tempGrace, orphanGrace time.Duration) (CleanupStats, error) {
	var stats CleanupStats
	var cleanupErrors []error
	for _, target := range []struct {
		prefix string
		grace  time.Duration
		temp   bool
	}{
		{prefix: ".tmp", grace: tempGrace, temp: true},
		{prefix: "documents", grace: orphanGrace},
	} {
		files, err := store.List(ctx, target.prefix)
		if err != nil {
			stats.Failed++
			cleanupErrors = append(cleanupErrors, fmt.Errorf("list %s files: %w", target.prefix, err))
			continue
		}
		for _, file := range files {
			stats.Scanned++
			if now.Sub(file.ModTime) < target.grace {
				stats.Skipped++
				continue
			}
			if !target.temp {
				if _, exists := referenced[file.Key]; exists {
					stats.Skipped++
					continue
				}
			}
			if err := store.Delete(ctx, file.Key); err != nil {
				stats.Failed++
				cleanupErrors = append(cleanupErrors, fmt.Errorf("delete %s: %w", file.Key, err))
				continue
			}
			stats.Deleted++
		}
	}
	return stats, errors.Join(cleanupErrors...)
}
