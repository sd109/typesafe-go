package qlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"

	"github.com/sd109/typesafe-go/internal/questioncli"
	"github.com/sd109/typesafe-go/pkg/sdk"
	corev1 "k8s.io/api/core/v1"
)

type podResult struct {
	Pod        corev1.Pod
	Containers []ContainerLog
	Response   *sdk.SystemOneResponse
}

type podAnswerResult struct {
	Namespace  string           `json:"namespace"`
	Resource   string           `json:"resource"`
	Containers []ContainerInfo  `json:"containers"`
	Questions  []questionResult `json:"questions"`
}

type questionResult struct {
	ID       string     `json:"id"`
	Question string     `json:"question"`
	Type     string     `json:"type"`
	Answer   sdk.Answer `json:"answer"`
}

type dryRunResult struct {
	Namespace  string          `json:"namespace"`
	Resource   string          `json:"resource"`
	Containers []ContainerInfo `json:"containers"`
}

func renderResults(writer io.Writer, output string, tableWidth int, allNamespaces bool, queries []questioncli.Query, results []podResult) error {
	if output == "json" {
		return renderJSON(writer, queries, results)
	}
	return renderTable(writer, tableWidth, allNamespaces, queries, results)
}

func renderTable(writer io.Writer, tableWidth int, allNamespaces bool, queries []questioncli.Query, results []podResult) error {
	var rendered bytes.Buffer
	table := tablewriter.NewTable(
		&rendered,
		tablewriter.WithMaxWidth(tableWidth),
		tablewriter.WithRendition(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		}),
	)
	showNamespace := allNamespaces
	showContainers := !allResultsHaveSingleContainer(results)
	headers := []string{"Resource"}
	if showNamespace {
		headers = append([]string{"Namespace"}, headers...)
	}
	if showContainers {
		headers = append(headers, "Containers")
	}
	if len(queries) == 1 {
		headers = append(headers, "Value", "Confidence")
	} else {
		headers = append(headers, "Type", "Value", "Confidence", "Question")
	}
	table.Header(tableHeaderValues(headers)...)
	for _, result := range results {
		containerSummary := summarizeContainers(result.Containers)
		for _, query := range queries {
			answer, err := questioncli.AnswerFor(query, result.Response)
			if err != nil {
				return err
			}
			value, confidence := questioncli.AnswerColumns(answer)
			row := []string{result.Pod.Name}
			if showNamespace {
				row = append([]string{result.Pod.Namespace}, row...)
			}
			if showContainers {
				row = append(row, containerSummary)
			}
			if len(queries) == 1 {
				row = append(row, value, confidence)
			} else {
				row = append(row, string(query.Kind), value, confidence, query.Question)
			}
			if err := table.Append(row); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
		}
	}
	if err := table.Render(); err != nil {
		return fmt.Errorf("render output: %w", err)
	}
	if _, err := writer.Write(rendered.Bytes()); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func renderJSON(writer io.Writer, queries []questioncli.Query, results []podResult) error {
	answers := make([]podAnswerResult, 0, len(results))
	for _, result := range results {
		questions := make([]questionResult, 0, len(queries))
		for _, query := range queries {
			answer, err := questioncli.AnswerFor(query, result.Response)
			if err != nil {
				return err
			}
			questions = append(questions, questionResult{
				ID:       query.ID,
				Question: query.Question,
				Type:     string(query.Kind),
				Answer:   answer,
			})
		}
		answers = append(answers, podAnswerResult{
			Namespace:  result.Pod.Namespace,
			Resource:   "pod/" + result.Pod.Name,
			Containers: containerInfos(result.Containers),
			Questions:  questions,
		})
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(answers); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}

func renderDryRun(writer io.Writer, output string, tableWidth int, allNamespaces bool, pods []corev1.Pod, options LogOptions) error {
	items := make([]dryRunResult, 0, len(pods))
	for index := range pods {
		containers, err := SelectContainers(&pods[index], options)
		if err != nil {
			return err
		}
		items = append(items, dryRunResult{
			Namespace:  pods[index].Namespace,
			Resource:   "pod/" + pods[index].Name,
			Containers: containers,
		})
	}
	if output == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(items); err != nil {
			return fmt.Errorf("write JSON output: %w", err)
		}
		return nil
	}

	var rendered bytes.Buffer
	table := tablewriter.NewTable(
		&rendered,
		tablewriter.WithMaxWidth(tableWidth),
		tablewriter.WithRendition(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		}),
	)
	showNamespace := allNamespaces
	showContainers := !allDryRunResultsHaveSingleContainer(items)
	headers := []string{"Resource"}
	if showNamespace {
		headers = append([]string{"Namespace"}, headers...)
	}
	if showContainers {
		headers = append(headers, "Containers")
	}
	table.Header(tableHeaderValues(headers)...)
	for _, item := range items {
		row := []string{strings.TrimPrefix(item.Resource, "pod/")}
		if showNamespace {
			row = append([]string{item.Namespace}, row...)
		}
		if showContainers {
			row = append(row, summarizeContainerInfos(item.Containers))
		}
		if err := table.Append(row); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	if err := table.Render(); err != nil {
		return fmt.Errorf("render output: %w", err)
	}
	if _, err := writer.Write(rendered.Bytes()); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func tableHeaderValues(headers []string) []any {
	values := make([]any, len(headers))
	for index, header := range headers {
		values[index] = header
	}
	return values
}

func allResultsHaveSingleContainer(results []podResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if len(result.Containers) != 1 {
			return false
		}
	}
	return true
}

func allDryRunResultsHaveSingleContainer(results []dryRunResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if len(result.Containers) != 1 {
			return false
		}
	}
	return true
}

func containerInfos(logs []ContainerLog) []ContainerInfo {
	result := make([]ContainerInfo, 0, len(logs))
	for _, log := range logs {
		result = append(result, log.ContainerInfo)
	}
	return result
}

func summarizeContainers(logs []ContainerLog) string {
	return summarizeContainerInfos(containerInfos(logs))
}

func summarizeContainerInfos(containers []ContainerInfo) string {
	values := make([]string, 0, len(containers))
	for _, container := range containers {
		values = append(values, container.Name)
	}
	return strings.Join(values, ", ")
}
