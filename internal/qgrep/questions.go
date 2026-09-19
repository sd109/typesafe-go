package qgrep

import (
	"fmt"
	"strings"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

type questionKind string

const (
	kindNoul   questionKind = sdk.NoulType
	kindChoice questionKind = sdk.ChoiceType
	kindScore  questionKind = sdk.ScoreType
)

type query struct {
	id       string
	kind     questionKind
	question string
}

func parseQuestions(noul, choice, score []string) (map[string]sdk.Question, []query, error) {
	questions := make(map[string]sdk.Question, len(noul)+len(choice)+len(score))
	queries := make([]query, 0, len(noul)+len(choice)+len(score))

	for index, value := range noul {
		question, err := parseNoul(value)
		if err != nil {
			return nil, nil, fmt.Errorf("--noul question %d: %w", index+1, err)
		}
		id := fmt.Sprintf("noul-%d", index+1)
		questions[id] = question
		queries = append(queries, query{id: id, kind: kindNoul, question: strings.TrimSpace(value)})
	}
	for index, value := range choice {
		question, err := parseChoice(value)
		if err != nil {
			return nil, nil, fmt.Errorf("--choice question %d: %w", index+1, err)
		}
		id := fmt.Sprintf("choice-%d", index+1)
		questions[id] = question
		queries = append(queries, query{id: id, kind: kindChoice, question: strings.TrimSpace(value)})
	}
	for index, value := range score {
		question, err := parseScore(value)
		if err != nil {
			return nil, nil, fmt.Errorf("--score question %d: %w", index+1, err)
		}
		id := fmt.Sprintf("score-%d", index+1)
		questions[id] = question
		queries = append(queries, query{id: id, kind: kindScore, question: strings.TrimSpace(value)})
	}
	if len(queries) == 0 {
		return nil, nil, fmt.Errorf("at least one of --noul, --choice, or --score is required")
	}
	return questions, queries, nil
}

func parseNoul(value string) (sdk.Question, error) {
	question := strings.TrimSpace(value)
	if question == "" {
		return nil, fmt.Errorf("question must not be empty")
	}
	return sdk.NoulQuestion{Instructions: question}, nil
}

func parseChoice(value string) (sdk.Question, error) {
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

func parseScore(value string) (sdk.Question, error) {
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
