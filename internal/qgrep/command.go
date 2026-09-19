package qgrep

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

// Client is the part of the TypeSafe client used by qgrep.
type Client interface {
	SystemOne(ctx context.Context, request sdk.SystemOneRequest, options ...sdk.RequestOption) (*sdk.SystemOneResponse, error)
	Close()
}

// ClientFactory creates the API client after local CLI validation succeeds.
type ClientFactory func() (Client, error)

// NewCommand constructs the qgrep Cobra command. A nil factory uses the
// default SDK client, which reads its configuration from the environment.
func NewCommand(factory ClientFactory) *cobra.Command {
	if factory == nil {
		factory = func() (Client, error) {
			return sdk.NewClient()
		}
	}

	var noul, choice, score []string
	var model string
	var tableWidth int
	var jsonOutput bool

	command := &cobra.Command{
		Use:           "qgrep",
		Short:         "Ask TypeSafe questions about stdin",
		Long:          "qgrep reads a document from stdin and evaluates natural-language questions with TypeSafe.",
		Version:       sdk.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !jsonOutput && tableWidth <= 0 {
				return fmt.Errorf("table width must be positive")
			}
			questions, queries, err := parseQuestions(noul, choice, score)
			if err != nil {
				return err
			}

			state, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			if strings.TrimSpace(string(state)) == "" {
				return fmt.Errorf("stdin is empty")
			}

			request := sdk.SystemOneRequest{
				State:     string(state),
				Questions: questions,
			}
			validationRequest := request
			validationRequest.Model = sdk.DefaultModel
			if err := validationRequest.Validate(); err != nil {
				return fmt.Errorf("build request: %w", err)
			}

			var options []sdk.RequestOption
			if strings.TrimSpace(model) != "" {
				options = append(options, sdk.WithRequestModel(model))
			}

			client, err := factory()
			if err != nil {
				return fmt.Errorf("create TypeSafe client: %w", err)
			}
			if client == nil {
				return fmt.Errorf("create TypeSafe client: factory returned a nil client")
			}
			defer client.Close()

			response, err := client.SystemOne(cmd.Context(), request, options...)
			if err != nil {
				return err
			}
			if response == nil {
				return fmt.Errorf("TypeSafe client returned a nil response")
			}
			if jsonOutput {
				return renderJSON(cmd.OutOrStdout(), queries, response)
			}
			return renderText(cmd.OutOrStdout(), tableWidth, queries, response)
		},
	}

	command.Flags().StringArrayVar(&noul, "noul", nil, "ask a yes/no question (repeatable)")
	command.Flags().StringArrayVar(&choice, "choice", nil, "ask a choice question using {option one, option two} (repeatable)")
	command.Flags().StringArrayVar(&score, "score", nil, "ask a score question using [low, medium, high] (repeatable)")
	command.Flags().StringVar(&model, "model", "", "TypeSafe model override")
	command.Flags().IntVar(&tableWidth, "table-width", 300, "maximum width of the human-readable table")
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit ordered JSON results")
	command.MarkFlagsOneRequired("noul", "choice", "score")

	return command
}
