//go:build !unix

package housekeep

func volume(string) (int64, int64, bool) { return 0, 0, false }
