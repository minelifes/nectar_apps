//go:build unix

package housekeep

import "syscall"

// volume returns the size and free space of the filesystem holding path.
func volume(path string) (total, free int64, ok bool) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, 0, false
	}
	return int64(s.Blocks) * int64(s.Bsize), int64(s.Bavail) * int64(s.Bsize), true
}
