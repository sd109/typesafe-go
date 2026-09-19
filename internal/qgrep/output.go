package qgrep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

type jsonResult struct {
	ID       string     `json:"id"`
	Question string     `json:"question"`
	Type     string     `json:"type"`
	Answer   sdk.Answer `json:"answer"`
}

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
	results := make([]jsonResult, 0, len(queries))
	for _, item := range queries {
		answer, err := answerFor(item, response)
		if err != nil {
			return err
		}
		results = append(results, jsonResult{
			ID:       item.id,
			Question: item.question,
			Type:     string(item.kind),
			Answer:   answer,
		})
	}

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}

func answerFor(item query, response *sdk.SystemOneResponse) (sdk.Answer, error) {
	answer, ok := response.Answers[item.id]
	if !ok || answer == nil || isNilAnswer(answer) {
		return nil, fmt.Errorf("TypeSafe response did not contain answer %q", item.id)
	}
	switch item.kind {
	case kindNoul:
		switch value := answer.(type) {
		case sdk.NoulAnswer:
			return value, nil
		case *sdk.NoulAnswer:
			return value, nil
		}
	case kindChoice:
		switch value := answer.(type) {
		case sdk.ChoiceAnswer:
			return value, nil
		case *sdk.ChoiceAnswer:
			return value, nil
		}
	case kindScore:
		switch value := answer.(type) {
		case sdk.ScoreAnswer:
			return value, nil
		case *sdk.ScoreAnswer:
			return value, nil
		}
	}
	return nil, fmt.Errorf("TypeSafe answer %q has type %T; expected %s", item.id, answer, item.kind)
}

func answerColumns(answer sdk.Answer) (string, string) {
	switch value := answer.(type) {
	case sdk.NoulAnswer:
		return formatNumber(value.Noul), ""
	case *sdk.NoulAnswer:
		return formatNumber(value.Noul), ""
	case sdk.ChoiceAnswer:
		return value.Choice, formatNumber(value.Confidence)
	case *sdk.ChoiceAnswer:
		return value.Choice, formatNumber(value.Confidence)
	case sdk.ScoreAnswer:
		return formatNumber(value.Score), formatNumber(value.Confidence)
	case *sdk.ScoreAnswer:
		return formatNumber(value.Score), formatNumber(value.Confidence)
	default:
		return "", ""
	}
}

func isNilAnswer(answer sdk.Answer) bool {
	value := reflect.ValueOf(answer)
	return value.IsValid() && value.Kind() == reflect.Pointer && value.IsNil()
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 4, 64)
}
