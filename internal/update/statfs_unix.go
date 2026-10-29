//go:build unix

package update

import "syscall"

// freeSpace returns the free bytes of the filesystem holding path.
func freeSpace(path string) (int64, bool) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, false
	}
	return int64(s.Bavail) * int64(s.Bsize), true
}
