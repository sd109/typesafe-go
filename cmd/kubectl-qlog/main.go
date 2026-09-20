package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func newCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "kubectl-qlog [resource...]",
		Short:         "Ask TypeSafe questions about Kubernetes pod logs",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("kubectl-qlog is not implemented yet")
		},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := newCommand()
	command.SetContext(ctx)
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
