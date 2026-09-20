package qgrep

import (
	"github.com/sd109/typesafe-go/internal/questioncli"
	"github.com/sd109/typesafe-go/pkg/sdk"
)

type questionKind = questioncli.Kind

const (
	kindNoul   = questioncli.KindNoul
	kindChoice = questioncli.KindChoice
	kindScore  = questioncli.KindScore
)

type query struct {
	id       string
	kind     questionKind
	question string
}

func parseQuestions(noul, choice, score []string) (map[string]sdk.Question, []query, error) {
	questions, parsed, err := questioncli.ParseQuestions(noul, choice, score)
	if err != nil {
		return nil, nil, err
	}
	queries := make([]query, 0, len(parsed))
	for _, item := range parsed {
		queries = append(queries, query{
			id:       item.ID,
			kind:     item.Kind,
			question: item.Question,
		})
	}
	return questions, queries, nil
}

func parseNoul(value string) (sdk.Question, error) {
	return questioncli.ParseNoul(value)
}

func parseChoice(value string) (sdk.Question, error) {
	return questioncli.ParseChoice(value)
}

func parseScore(value string) (sdk.Question, error) {
	return questioncli.ParseScore(value)
}
