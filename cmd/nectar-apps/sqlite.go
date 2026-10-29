package main

// Pure-Go SQLite driver (no CGO), so the binary is static and cross-compiles for amd64/arm64.
import _ "modernc.org/sqlite"

const sqliteDriver = "sqlite"
