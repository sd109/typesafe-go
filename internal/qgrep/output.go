package qgrep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"

	"github.com/sd109/typesafe-go/internal/questioncli"
	"github.com/sd109/typesafe-go/pkg/sdk"
)

type jsonResult = questioncli.JSONResult

func renderText(writer io.Writer, tableWidth int, queries []query, response *sdk.SystemOneResponse) error {
	var rendered bytes.Buffer
	table := tablewriter.NewTable(
		&rendered,
		tablewriter.WithMaxWidth(tableWidth),
		tablewriter.WithRendition(tw.Rendition{
			Settings: tw.Settings{
				Separators: tw.Separators{BetweenRows: tw.On},
			},
		}),
	)
	table.Header("Type", "Value", "Confidence", "Question")
	for _, item := range queries {
		answer, err := answerFor(item, response)
		if err != nil {
			return err
		}
		primary, confidence := answerColumns(answer)
		if err := table.Append([]string{string(item.kind), primary, confidence, item.question}); err != nil {
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

func renderJSON(writer io.Writer, queries []query, response *sdk.SystemOneResponse) error {
	sharedQueries := make([]questioncli.Query, 0, len(queries))
	for _, item := range queries {
		sharedQueries = append(sharedQueries, questioncli.Query{
			ID:       item.id,
			Kind:     item.kind,
			Question: item.question,
		})
	}
	results, err := questioncli.BuildJSONResults(sharedQueries, response)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}

func answerFor(item query, response *sdk.SystemOneResponse) (sdk.Answer, error) {
	return questioncli.AnswerFor(questioncli.Query{
		ID:       item.id,
		Kind:     item.kind,
		Question: item.question,
	}, response)
}

func answerColumns(answer sdk.Answer) (string, string) {
	return questioncli.AnswerColumns(answer)
}

func formatNumber(value float64) string {
	return questioncli.FormatNumber(value)
}
