//go:build windows

package runner

// freeBytes is not checked on Windows; the download fails cleanly if the disk fills.
func freeBytes(string) (int64, bool) { return 0, false }
