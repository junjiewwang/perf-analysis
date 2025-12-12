package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/perf-analysis/internal/webui"
	"github.com/perf-analysis/pkg/utils"
)

var (
	// Serve command flags
	dataDir string
	port    int
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web server to view analysis results",
	Long: `Start an HTTP server to interactively view and explore analysis results.

The serve command starts a lightweight web server that provides:
  - Interactive flame graph visualization
  - Top functions analysis
  - Thread statistics
  - Task switching between multiple analyses

The web UI uses d3-flame-graph for rendering interactive flame graphs
that support zooming, searching, and detailed tooltips.`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Set dynamic example using actual binary name
	binName := BinName()
	serveCmd.Example = `  # Start server with default settings (port 8080, ./output directory)
  ` + binName + ` serve

  # Specify data directory and port
  ` + binName + ` serve -d ./my-output -p 9090

  # Start server with verbose logging
  ` + binName + ` serve -d ./output -v`

	serveCmd.Flags().StringVarP(&dataDir, "data-dir", "d", "./output", "Data directory containing analysis results")
	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port for web server")
}

func runServe(cmd *cobra.Command, args []string) error {
	log := GetLogger()
	return startServeMode(dataDir, port, log)
}

// startServeMode is shared between analyze --serve and serve command
func startServeMode(dataDirectory string, serverPort int, log utils.Logger) error {
	// Verify data directory exists
	if _, err := os.Stat(dataDirectory); os.IsNotExist(err) {
		return fmt.Errorf("data directory not found: %s", dataDirectory)
	}

	server := webui.NewServer(dataDirectory, serverPort, log)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("\nShutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		server.Shutdown(ctx)
		os.Exit(0)
	}()

	// Print access URL
	log.Info("")
	log.Info("╔════════════════════════════════════════════════════════╗")
	log.Info("║  🔥 Perf Analysis Viewer                               ║")
	log.Info("║                                                        ║")
	log.Info("║  Open in browser: http://localhost:%-5d               ║", serverPort)
	log.Info("║  Data directory:  %-36s ║", truncateString(dataDirectory, 36))
	log.Info("║                                                        ║")
	log.Info("║  Press Ctrl+C to stop                                  ║")
	log.Info("╚════════════════════════════════════════════════════════╝")
	log.Info("")

	if err := server.Start(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
