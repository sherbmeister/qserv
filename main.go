package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := LoadConfigFromFlagsAndEnv()
	logger := NewLogger(cfg.LogLevel)

	// Optional JSON overrides
	if cfg.ConfigFile != "" {
		fileCfg, err := LoadConfigFromFile(cfg.ConfigFile)
		if err != nil {
			logger.Fatalf("failed to load config file %s: %v", cfg.ConfigFile, err)
		}
		cfg = cfg.Merge(fileCfg)
	}

	logger.Infof("starting qserv — linking to %s:%d (%s)", cfg.Host, cfg.Port, map[bool]string{true: "TLS", false: "TCP"}[cfg.TLS])

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Warnf("shutdown requested")
		cancel()
	}()

	link := &Link{
		Cfg:    cfg,
		Logger: logger,
		Bus:    NewBus(logger),
	}

	// Reconnect loop
	backoff := time.Second
	for ctx.Err() == nil {
		if err := link.ConnectAndRun(ctx); err != nil {
			logger.Errorf("link error: %v — reconnecting in %s", err, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		break
	}

	logger.Infof("qserv stopped")
}
