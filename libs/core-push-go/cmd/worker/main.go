package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lzyats/core-push-go/pkg/push"
	"github.com/lzyats/core-push-go/pkg/runner"
	"gopkg.in/yaml.v3"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", os.Getenv("CORE_PUSH_CONFIG"), "config yml path")
	flag.Parse()
	if cfgPath == "" {
		log.Fatal("missing -config or CORE_PUSH_CONFIG")
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	var st push.Settings
	if err := yaml.Unmarshal(b, &st); err != nil {
		log.Fatal(err)
	}
	st = st.WithDefaults()

	w, err := runner.NewWorker(st)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := w.Run(ctx); err != nil {
		log.Printf("worker stopped: %v", err)
	}
}
