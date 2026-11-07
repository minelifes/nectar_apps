//go:build !unix

package api

// freeDiskBytes is unknown on this platform (-1).
func freeDiskBytes(string) int64 { return -1 }
