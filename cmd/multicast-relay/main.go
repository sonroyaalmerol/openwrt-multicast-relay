package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sonroyaalmerol/openwrt-multicast-relay/internal/relay"
)

var version = "dev"

func main() {
	cfg, err := relay.ParseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "multicast-relay %s: %v\n", version, err)
		os.Exit(1)
	}

	r, err := relay.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "multicast-relay %s: %v\n", version, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := r.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "multicast-relay %s: %v\n", version, err)
		os.Exit(1)
	}
}
