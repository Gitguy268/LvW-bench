package runner

import (
	"fmt"

	"github.com/Gitguy268/LvW-bench/internal/hub"
)

// checkDisk refuses a download that would leave less than 2 GB free in the HF cache.
func checkDisk(need int64) error {
	free, ok := freeBytes(hub.CacheDir())
	if !ok || need <= 0 {
		return nil
	}
	const margin = 2 << 30
	if free-need < margin {
		return fmt.Errorf("not enough disk space: the model needs %.1f GB and only %.1f GB is free in %s (set HF_HUB_CACHE to use another disk)",
			float64(need)/1e9, float64(free)/1e9, hub.CacheDir())
	}
	return nil
}
