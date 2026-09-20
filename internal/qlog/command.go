package qlog

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sd109/typesafe-go/internal/questioncli"
	"github.com/sd109/typesafe-go/pkg/sdk"
	corev1 "k8s.io/api/core/v1"
)

// TypeSafeClient is the part of the SDK used by qlog.
type TypeSafeClient interface {
	SystemOne(context.Context, sdk.SystemOneRequest, ...sdk.RequestOption) (*sdk.SystemOneResponse, error)
	Close()
}

// CommandDependencies makes command execution testable without a cluster or
// TypeSafe credentials.
type CommandDependencies struct {
	Config                  *ConfigOptions
	KubernetesClientFactory func(*ConfigOptions) (KubernetesClient, error)
	ResolverFactory         func(KubernetesClient) PodResolver
	LogFetcherFactory       func(KubernetesClient) LogFetcher
	TypeSafeClientFactory   func() (TypeSafeClient, error)
}

// PodResolver resolves command-line selections into concrete pods.
type PodResolver interface {
	Resolve(context.Context, NamespaceScope, []ResourceRef, bool) ([]corev1.Pod, error)
}

// NewCommand constructs the kubectl-qlog command.
func NewCommand(dependencies CommandDependencies) *cobra.Command {
	if dependencies.Config == nil {
		dependencies.Config = NewConfigOptions()
	}
	if dependencies.KubernetesClientFactory == nil {
		dependencies.KubernetesClientFactory = func(options *ConfigOptions) (KubernetesClient, error) {
			return options.Client()
		}
	}
	if dependencies.ResolverFactory == nil {
		dependencies.ResolverFactory = func(client KubernetesClient) PodResolver {
			return NewResolver(client)
		}
	}
	if dependencies.LogFetcherFactory == nil {
		dependencies.LogFetcherFactory = func(client KubernetesClient) LogFetcher {
			return NewLogFetcher(client)
		}
	}
	if dependencies.TypeSafeClientFactory == nil {
		dependencies.TypeSafeClientFactory = func() (TypeSafeClient, error) {
			return sdk.NewClient()
		}
	}

	var (
		noul, choice, score []string
		output              string
		allPods             bool
		dryRun              bool
		parallelism         int
		tableWidth          int
		since               time.Duration
		sinceTime           string
		tail                int64 = -1
		container           string
		allContainers       bool
	)

	command := &cobra.Command{
		Use:           "kubectl-qlog [resource...]",
		Short:         "Ask TypeSafe questions about Kubernetes pod logs",
		Long:          "kubectl-qlog resolves Kubernetes resources to pods, evaluates their logs, and asks TypeSafe natural-language questions.",
		Version:       sdk.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommandFlags(output, parallelism, tableWidth); err != nil {
				return err
			}
			logOptions := LogOptions{
				Since:         since,
				SinceTime:     sinceTime,
				Tail:          tail,
				Container:     container,
				AllContainers: allContainers,
			}
			if err := logOptions.Validate(); err != nil {
				return err
			}

			refs, err := parseResourceArgs(args)
			if err != nil {
				return err
			}
			var questions map[string]sdk.Question
			var queries []questioncli.Query
			if !dryRun || hasQuestions(noul, choice, score) {
				questions, queries, err = questioncli.ParseQuestions(noul, choice, score)
				if err != nil && dryRun && !hasQuestions(noul, choice, score) {
					err = nil
				}
				if err != nil {
					return err
				}
			}

			scope, err := dependencies.Config.NamespaceScope(cmd.Flags())
			if err != nil {
				return err
			}
			if allPods && len(refs) > 0 {
				return fmt.Errorf("--all-pods cannot be combined with resource arguments")
			}
			if !allPods && len(refs) == 0 {
				return fmt.Errorf("a resource argument or --all-pods is required")
			}

			kubernetesClient, err := dependencies.KubernetesClientFactory(dependencies.Config)
			if err != nil {
				return fmt.Errorf("create Kubernetes client: %w", err)
			}
			if kubernetesClient == nil {
				return fmt.Errorf("create Kubernetes client: factory returned a nil client")
			}
			resolver := dependencies.ResolverFactory(kubernetesClient)
			if resolver == nil {
				return fmt.Errorf("create pod resolver: factory returned a nil resolver")
			}
			pods, err := resolver.Resolve(cmd.Context(), scope, refs, allPods)
			if err != nil {
				return err
			}
			sort.SliceStable(pods, func(i, j int) bool {
				if pods[i].Namespace != pods[j].Namespace {
					return pods[i].Namespace < pods[j].Namespace
				}
				return pods[i].Name < pods[j].Name
			})
			if dryRun {
				return renderDryRun(cmd.OutOrStdout(), output, tableWidth, scope.All, pods, logOptions)
			}

			client, err := dependencies.TypeSafeClientFactory()
			if err != nil {
				return fmt.Errorf("create TypeSafe client: %w", err)
			}
			if client == nil {
				return fmt.Errorf("create TypeSafe client: factory returned a nil client")
			}
			defer client.Close()

			fetcher := dependencies.LogFetcherFactory(kubernetesClient)
			if fetcher == nil {
				return fmt.Errorf("create pod log fetcher: factory returned a nil fetcher")
			}
			results, err := processPods(cmd.Context(), pods, logOptions, parallelism, questions, fetcher, client)
			if err != nil {
				return err
			}
			return renderResults(cmd.OutOrStdout(), output, tableWidth, scope.All, queries, results)
		},
	}

	dependencies.Config.AddFlags(command.Flags())
	command.Flags().BoolVar(&allPods, "all-pods", false, "Get logs from all pods in the selected namespace scope.")
	command.Flags().StringArrayVar(&noul, "noul", nil, "ask a yes/no question (repeatable)")
	command.Flags().StringArrayVar(&choice, "choice", nil, "ask a choice question using {option one, option two} (repeatable)")
	command.Flags().StringArrayVar(&score, "score", nil, "ask a score question using [low, medium, high] (repeatable)")
	command.Flags().StringVarP(&output, "output", "o", "table", "output format: table or json")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "resolve and display pods and containers without fetching logs or calling TypeSafe")
	command.Flags().IntVar(&parallelism, "parallelism", 4, "maximum number of pods processed concurrently")
	command.Flags().IntVar(&tableWidth, "table-width", 300, "maximum width of human-readable table output")
	command.Flags().DurationVar(&since, "since", 0, "only return logs newer than a relative duration such as 5m or 2h")
	command.Flags().StringVar(&sinceTime, "since-time", "", "only return logs after an RFC3339 timestamp")
	command.Flags().Int64Var(&tail, "tail", -1, "lines of recent log file to display; -1 means all lines")
	command.Flags().StringVarP(&container, "container", "c", "", "container name")
	command.Flags().BoolVar(&allContainers, "all-containers", false, "get logs from all init, regular, and ephemeral containers")
	setGroupedHelp(command)

	return command
}

var qlogHelpFlags = []string{
	"all-containers",
	"all-pods",
	"choice",
	"container",
	"dry-run",
	"noul",
	"output",
	"parallelism",
	"score",
	"since",
	"since-time",
	"tail",
	"table-width",
}

func setGroupedHelp(command *cobra.Command) {
	command.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		writer := cmd.OutOrStdout()
		fmt.Fprintf(writer, "Usage:\n  %s\n\n", cmd.UseLine())
		if cmd.Long != "" {
			fmt.Fprintln(writer, cmd.Long)
		} else {
			fmt.Fprintln(writer, cmd.Short)
		}
		fmt.Fprintln(writer)
		writeFlagGroup(writer, cmd, "Qlog options", qlogHelpFlags)

		qlogNames := make(map[string]struct{}, len(qlogHelpFlags))
		for _, name := range qlogHelpFlags {
			qlogNames[name] = struct{}{}
		}
		standard := make([]string, 0)
		general := make([]string, 0)
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if _, ok := qlogNames[flag.Name]; ok {
				return
			}
			switch flag.Name {
			case "help", "version":
				general = append(general, flag.Name)
			default:
				standard = append(standard, flag.Name)
			}
		})
		writeFlagGroup(writer, cmd, "Kubernetes configuration", standard)
		writeFlagGroup(writer, cmd, "General options", general)
	})
}

func writeFlagGroup(writer io.Writer, command *cobra.Command, title string, names []string) {
	if len(names) == 0 {
		return
	}
	flags := pflag.NewFlagSet(command.Name(), pflag.ContinueOnError)
	flags.SortFlags = false
	for _, name := range names {
		if flag := command.Flags().Lookup(name); flag != nil {
			flags.AddFlag(flag)
		}
	}
	usage := flags.FlagUsagesWrapped(0)
	if usage == "" {
		return
	}
	fmt.Fprintf(writer, "%s:\n%s\n", title, usage)
}

func validateCommandFlags(output string, parallelism, tableWidth int) error {
	if output != "table" && output != "json" {
		return fmt.Errorf("unsupported output format %q; expected table or json", output)
	}
	if parallelism <= 0 {
		return fmt.Errorf("--parallelism must be positive")
	}
	if output != "json" && tableWidth <= 0 {
		return fmt.Errorf("table width must be positive")
	}
	return nil
}

func parseResourceArgs(args []string) ([]ResourceRef, error) {
	refs := make([]ResourceRef, 0, len(args))
	for _, arg := range args {
		ref, err := ParseResourceRef(arg)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func hasQuestions(noul, choice, score []string) bool {
	return len(noul) > 0 || len(choice) > 0 || len(score) > 0
}

func processPods(ctx context.Context, pods []corev1.Pod, options LogOptions, parallelism int, questions map[string]sdk.Question, fetcher LogFetcher, client TypeSafeClient) ([]podResult, error) {
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found")
	}
	if parallelism > len(pods) {
		parallelism = len(pods)
	}
	workContext, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]podResult, len(pods))
	jobs := make(chan int)
	var workers sync.WaitGroup
	var firstErr error
	var failOnce sync.Once
	fail := func(err error) {
		if err == nil {
			return
		}
		failOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	for worker := 0; worker < parallelism; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-workContext.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					pod := &pods[index]
					containers, err := fetcher.Fetch(workContext, pod, options)
					if err != nil {
						fail(fmt.Errorf("process pod %s/%s: %w", pod.Namespace, pod.Name, err))
						return
					}
					state := podState(*pod, containers)
					request := sdk.SystemOneRequest{State: state, Questions: questions, Model: sdk.DefaultModel}
					if err := request.Validate(); err != nil {
						fail(fmt.Errorf("build TypeSafe request for pod %s/%s: %w", pod.Namespace, pod.Name, err))
						return
					}
					response, err := client.SystemOne(workContext, request)
					if err != nil {
						fail(fmt.Errorf("process pod %s/%s: %w", pod.Namespace, pod.Name, err))
						return
					}
					if response == nil {
						fail(fmt.Errorf("TypeSafe client returned a nil response for pod %s/%s", pod.Namespace, pod.Name))
						return
					}
					results[index] = podResult{Pod: *pod, Containers: containers, Response: response}
				}
			}
		}()
	}

	for index := range pods {
		select {
		case <-workContext.Done():
			break
		case jobs <- index:
		}
		if workContext.Err() != nil {
			break
		}
	}
	close(jobs)
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func podState(pod corev1.Pod, logs []ContainerLog) map[string]any {
	containers := make([]any, 0, len(logs))
	for _, item := range logs {
		containers = append(containers, map[string]any{
			"name":  item.Name,
			"type":  item.Type,
			"image": item.Image,
			"logs":  item.Logs,
		})
	}
	return map[string]any{
		"namespace":  pod.Namespace,
		"resource":   "pod/" + pod.Name,
		"containers": containers,
	}
}
