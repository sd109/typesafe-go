package questioncli

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

// JSONResult is the stable machine-readable representation of one answer.
type JSONResult struct {
	ID       string     `json:"id"`
	Question string     `json:"question"`
	Type     string     `json:"type"`
	Answer   sdk.Answer `json:"answer"`
}

// AnswerFor validates and returns the answer for a query. It rejects missing,
// nil, and type-mismatched answers before a caller renders any output.
func AnswerFor(query Query, response *sdk.SystemOneResponse) (sdk.Answer, error) {
	if response == nil {
		return nil, fmt.Errorf("TypeSafe response was nil while looking for answer %q", query.ID)
	}
	answer, ok := response.Answers[query.ID]
	if !ok || answer == nil || isNilAnswer(answer) {
		return nil, fmt.Errorf("TypeSafe response did not contain answer %q", query.ID)
	}
	switch query.Kind {
	case KindNoul:
		switch value := answer.(type) {
		case sdk.NoulAnswer:
			return value, nil
		case *sdk.NoulAnswer:
			return value, nil
		}
	case KindChoice:
		switch value := answer.(type) {
		case sdk.ChoiceAnswer:
			return value, nil
		case *sdk.ChoiceAnswer:
			return value, nil
		}
	case KindScore:
		switch value := answer.(type) {
		case sdk.ScoreAnswer:
			return value, nil
		case *sdk.ScoreAnswer:
			return value, nil
		}
	}
	return nil, fmt.Errorf("TypeSafe answer %q has type %T; expected %s", query.ID, answer, query.Kind)
}

// AnswerColumns returns the primary value and confidence strings used by the
// human-readable qgrep-style table.
func AnswerColumns(answer sdk.Answer) (string, string) {
	switch value := answer.(type) {
	case sdk.NoulAnswer:
		return FormatNumber(value.Noul), ""
	case *sdk.NoulAnswer:
		return FormatNumber(value.Noul), ""
	case sdk.ChoiceAnswer:
		return value.Choice, FormatNumber(value.Confidence)
	case *sdk.ChoiceAnswer:
		return value.Choice, FormatNumber(value.Confidence)
	case sdk.ScoreAnswer:
		return FormatNumber(value.Score), FormatNumber(value.Confidence)
	case *sdk.ScoreAnswer:
		return FormatNumber(value.Score), FormatNumber(value.Confidence)
	default:
		return "", ""
	}
}

// BuildJSONResults validates answers and returns them in query order.
func BuildJSONResults(queries []Query, response *sdk.SystemOneResponse) ([]JSONResult, error) {
	results := make([]JSONResult, 0, len(queries))
	for _, query := range queries {
		answer, err := AnswerFor(query, response)
		if err != nil {
			return nil, err
		}
		results = append(results, JSONResult{
			ID:       query.ID,
			Question: query.Question,
			Type:     string(query.Kind),
			Answer:   answer,
		})
	}
	return results, nil
}

// FormatNumber formats numeric answer fields consistently across CLI output.
func FormatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 4, 64)
}

func isNilAnswer(answer sdk.Answer) bool {
	value := reflect.ValueOf(answer)
	return value.IsValid() && value.Kind() == reflect.Pointer && value.IsNil()
}
