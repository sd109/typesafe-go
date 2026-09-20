// Package questioncli contains reusable TypeSafe question parsing and answer
// formatting helpers for command-line clients.
package questioncli

import (
	"fmt"
	"strings"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

// Kind identifies the TypeSafe question and answer shape.
type Kind string

const (
	KindNoul   Kind = sdk.NoulType
	KindChoice Kind = sdk.ChoiceType
	KindScore  Kind = sdk.ScoreType
)

// Query preserves the user-facing question order independently of the map used
// by the TypeSafe request.
type Query struct {
	ID       string
	Kind     Kind
	Question string
}

// ParseQuestions converts repeatable CLI question flags into a TypeSafe
// question map and an ordered description of those questions. Questions are
// returned in noul, choice, score order, preserving order within each type.
func ParseQuestions(noul, choice, score []string) (map[string]sdk.Question, []Query, error) {
	questions := make(map[string]sdk.Question, len(noul)+len(choice)+len(score))
	queries := make([]Query, 0, len(noul)+len(choice)+len(score))

	for index, value := range noul {
		question, err := ParseNoul(value)
		if err != nil {
			return nil, nil, fmt.Errorf("--noul question %d: %w", index+1, err)
		}
		id := fmt.Sprintf("noul-%d", index+1)
		questions[id] = question
		queries = append(queries, Query{ID: id, Kind: KindNoul, Question: strings.TrimSpace(value)})
	}
	for index, value := range choice {
		question, err := ParseChoice(value)
		if err != nil {
			return nil, nil, fmt.Errorf("--choice question %d: %w", index+1, err)
		}
		id := fmt.Sprintf("choice-%d", index+1)
		questions[id] = question
		queries = append(queries, Query{ID: id, Kind: KindChoice, Question: strings.TrimSpace(value)})
	}
	for index, value := range score {
		question, err := ParseScore(value)
		if err != nil {
			return nil, nil, fmt.Errorf("--score question %d: %w", index+1, err)
		}
		id := fmt.Sprintf("score-%d", index+1)
		questions[id] = question
		queries = append(queries, Query{ID: id, Kind: KindScore, Question: strings.TrimSpace(value)})
	}
	if len(queries) == 0 {
		return nil, nil, fmt.Errorf("at least one of --noul, --choice, or --score is required")
	}
	return questions, queries, nil
}

// ParseNoul parses a yes/no question.
func ParseNoul(value string) (sdk.Question, error) {
	question := strings.TrimSpace(value)
	if question == "" {
		return nil, fmt.Errorf("question must not be empty")
	}
	return sdk.NoulQuestion{Instructions: question}, nil
}

// ParseChoice parses a question containing one {comma-separated criteria}
// group.
func ParseChoice(value string) (sdk.Question, error) {
	question := strings.TrimSpace(value)
	if question == "" {
		return nil, fmt.Errorf("question must not be empty")
	}
	if strings.ContainsAny(question, "[]") {
		return nil, fmt.Errorf("choice criteria must use exactly one {comma-separated list}; square brackets are not allowed")
	}
	criteriaText, err := delimitedText(question, '{', '}')
	if err != nil {
		return nil, fmt.Errorf("invalid choice criteria: %w", err)
	}
	criteria, err := splitCriteria(criteriaText)
	if err != nil {
		return nil, fmt.Errorf("invalid choice criteria: %w", err)
	}
	values := make(map[string]sdk.JSONContent, len(criteria))
	for _, criterion := range criteria {
		values[criterion] = nil
	}
	return sdk.ChoiceQuestion{Instructions: question, Criteria: values}, nil
}

// ParseScore parses a question containing one [comma-separated criteria]
// group.
func ParseScore(value string) (sdk.Question, error) {
	question := strings.TrimSpace(value)
	if question == "" {
		return nil, fmt.Errorf("question must not be empty")
	}
	if strings.ContainsAny(question, "{}") {
		return nil, fmt.Errorf("score criteria must use exactly one [comma-separated list]; curly braces are not allowed")
	}
	criteriaText, err := delimitedText(question, '[', ']')
	if err != nil {
		return nil, fmt.Errorf("invalid score criteria: %w", err)
	}
	criteria, err := splitCriteria(criteriaText)
	if err != nil {
		return nil, fmt.Errorf("invalid score criteria: %w", err)
	}
	values := make([]sdk.JSONContent, len(criteria))
	for index, criterion := range criteria {
		values[index] = criterion
	}
	return sdk.ScoreQuestion{Instructions: question, Criteria: values}, nil
}

func delimitedText(value string, open, close byte) (string, error) {
	start := -1
	end := -1
	depth := 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case open:
			if depth > 0 {
				return "", fmt.Errorf("nested %c...%c groups are not supported", open, close)
			}
			if start >= 0 {
				return "", fmt.Errorf("only one %c...%c group is allowed", open, close)
			}
			start = index
			depth = 1
		case close:
			if depth == 0 {
				return "", fmt.Errorf("unexpected closing %c", close)
			}
			depth = 0
			end = index
		}
	}
	if start < 0 {
		return "", fmt.Errorf("missing %c...%c group", open, close)
	}
	if depth != 0 {
		return "", fmt.Errorf("missing closing %c", close)
	}
	return value[start+1 : end], nil
}

func splitCriteria(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	criteria := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		criterion := strings.TrimSpace(part)
		if criterion == "" {
			return nil, fmt.Errorf("criteria must not be empty")
		}
		if _, exists := seen[criterion]; exists {
			return nil, fmt.Errorf("criterion %q is duplicated", criterion)
		}
		seen[criterion] = struct{}{}
		criteria = append(criteria, criterion)
	}
	return criteria, nil
}
