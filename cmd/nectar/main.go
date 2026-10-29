// Command nectar is the Nectar Apps command-line client (deploys from a terminal or CI).
//
//	nectar login --url https://panel.example.com
//	nectar deploy shop/api --file target/app.jar
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"nectar_apps/internal/cli"
	"nectar_apps/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], cli.Env{Version: version.Version})
	stop()
	os.Exit(code)
}
