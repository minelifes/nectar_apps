//go:build !unix

package update

func freeSpace(string) (int64, bool) { return 0, false }
