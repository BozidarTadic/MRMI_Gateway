package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"MRMI_Gateway/internal/app"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/version"
)

func main() {
	var (
		configPath  string
		httpAddr    string
		showVersion bool
	)

	flag.StringVar(&configPath, "config", "", "Path to node configuration file")
	flag.StringVar(&httpAddr, "http-addr", "", "Override HTTP listen address")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("mrmi-gateway %s (ADR %s)\n", version.App, version.ADR)
		os.Exit(0)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if httpAddr != "" {
		cfg.Network.HTTPListenAddr = httpAddr
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, configPath); err != nil {
		log.Fatalf("run gateway: %v", err)
	}
}
