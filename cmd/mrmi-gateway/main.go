package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"MRMI_Gateway/internal/app"
	"MRMI_Gateway/internal/config"
)

func main() {
	var (
		configPath string
		httpAddr   string
	)

	flag.StringVar(&configPath, "config", "", "Path to node configuration file")
	flag.StringVar(&httpAddr, "http-addr", "", "Override HTTP listen address")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if httpAddr != "" {
		cfg.Network.HTTPListenAddr = httpAddr
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg); err != nil {
		log.Fatalf("run gateway: %v", err)
	}
}
