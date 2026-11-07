//go:build unix

package api

import "syscall"

// freeDiskBytes returns the space available to the panel in dir's filesystem (-1 if unknown).
func freeDiskBytes(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1
	}
	return int64(uint64(st.Bavail) * uint64(st.Bsize)) //nolint:gosec // block counts fit
}
