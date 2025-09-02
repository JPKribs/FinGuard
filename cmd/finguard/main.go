package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/version"
)

// MARK: main
// Main application entry point
func main() {
	var (
		configPath  = flag.String("config", "config.yaml", "Path to configuration file")
		versionFlag = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *versionFlag {
		fmt.Printf("FinGuard v%s\n", version.AsString())
		os.Exit(0)
	}

	for {
		internal.SetRestartFlag(false)

		if err := runApplication(*configPath); err != nil {
			log.Fatalf("Application error: %v", err)
		}

		if !internal.ShouldRestart() {
			break
		}

		log.Println("Restarting application...")
		time.Sleep(2 * time.Second)
	}
}

// MARK: runApplication
// Runs the complete application lifecycle
func runApplication(configPath string) error {
	app, err := newApplication(configPath)
	if err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	app.context = ctx
	app.cancel = cancel
	defer cancel()

	if err := app.start(ctx); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	app.handleSignals()
	app.waitGroup.Wait()

	return nil
}
