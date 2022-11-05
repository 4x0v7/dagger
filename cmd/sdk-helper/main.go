package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dagger/dagger/engine"
	"github.com/dagger/dagger/router"
	"github.com/dagger/dagger/tracing"
	"github.com/spf13/cobra"
)

var (
	configPath string
	workdir    string
	remote     string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&workdir, "workdir", "", "")
	rootCmd.PersistentFlags().StringVarP(&configPath, "project", "p", "", "")
	rootCmd.PersistentFlags().StringVar(&remote, "remote", "", "")
}

var rootCmd = &cobra.Command{
	Use: "", // TODO: message indicating not for human consumption
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		workdir, configPath, err = engine.NormalizePaths(workdir, configPath)
		return err
	},
	Run: Helper,
}

func Helper(cmd *cobra.Command, args []string) {
	startOpts := &engine.Config{
		Workdir:    workdir,
		ConfigPath: configPath,
		LogOutput:  os.Stderr,
		RemoteAddr: remote,
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	// shutdown if requested via signal
	go func() {
		<-signalCh
		l.Close()
	}()

	// shutdown if our parent closes stdin
	go func() {
		io.Copy(io.Discard, os.Stdin)
		l.Close()
	}()

	port := l.Addr().(*net.TCPAddr).Port

	err = engine.Start(context.Background(), startOpts, func(ctx context.Context, r *router.Router) error {
		srv := http.Server{
			Handler:           r,
			ReadHeaderTimeout: 30 * time.Second,
		}

		// TODO: still kind of racy, client should retry connections a few times
		go func() {
			if _, err := os.Stdout.Write([]byte(fmt.Sprintf("%d\n", port))); err != nil {
				panic(err)
			}
		}()

		return srv.Serve(l)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	closer := tracing.Init()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		closer.Close()
		os.Exit(1)
	}
	closer.Close()
}
