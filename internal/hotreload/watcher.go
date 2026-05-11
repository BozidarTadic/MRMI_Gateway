package hotreload

import (
	"context"
	"log"
	"os"
	"time"

	"MRMI_Gateway/internal/config"
)

type Watcher struct{}

func New() *Watcher { return &Watcher{} }

// Watch polls path every 500 ms. When the file modification time changes,
// it reloads and validates the config. On success it calls onChange;
// on failure it logs the error and retains the previous config.
// Returns when ctx is cancelled.
func (w *Watcher) Watch(ctx context.Context, path string, onChange func(config.Config)) {
	var lastMod time.Time
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			mod := info.ModTime()
			if lastMod.IsZero() {
				lastMod = mod
				continue
			}
			if mod.Equal(lastMod) {
				continue
			}
			lastMod = mod

			cfg, err := config.Load(path)
			if err != nil {
				log.Printf("[hotreload] config reload error: %v — keeping previous config", err)
				continue
			}
			onChange(cfg)
		}
	}
}
