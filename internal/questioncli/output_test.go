package questioncli

import (
	"strings"
	"testing"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

func TestAnswerForValidatesAnswerTypes(t *testing.T) {
	response := &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1":   sdk.NoulAnswer{Noul: 0.75},
		"choice-1": sdk.ChoiceAnswer{Choice: "api", Confidence: 0.8},
		"score-1":  sdk.ScoreAnswer{Score: 1.25, Confidence: 0.9},
	}}
	queries := []Query{
		{ID: "noul-1", Kind: KindNoul},
		{ID: "choice-1", Kind: KindChoice},
		{ID: "score-1", Kind: KindScore},
	}
	for _, query := range queries {
		if _, err := AnswerFor(query, response); err != nil {
			t.Errorf("AnswerFor(%q) error = %v", query.ID, err)
		}
	}

	if _, err := AnswerFor(Query{ID: "missing", Kind: KindNoul}, response); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing answer error = %v", err)
	}
	if _, err := AnswerFor(Query{ID: "choice-1", Kind: KindNoul}, response); err == nil || !strings.Contains(err.Error(), "expected noul") {
		t.Fatalf("wrong answer error = %v", err)
	}
	if _, err := AnswerFor(Query{ID: "nil", Kind: KindNoul}, &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"nil": (*sdk.NoulAnswer)(nil),
	}}); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("nil answer error = %v", err)
	}
	if _, err := AnswerFor(Query{ID: "n", Kind: KindNoul}, nil); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("nil response error = %v", err)
	}
}

func TestBuildJSONResultsPreservesQueryOrder(t *testing.T) {
	queries := []Query{
		{ID: "choice-1", Kind: KindChoice, Question: "choose"},
		{ID: "noul-1", Kind: KindNoul, Question: "is it"},
	}
	response := &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1":   sdk.NoulAnswer{Noul: 0.5},
		"choice-1": sdk.ChoiceAnswer{Choice: "a", Confidence: 0.7},
	}}
	results, err := BuildJSONResults(queries, response)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ID != "choice-1" || results[1].ID != "noul-1" {
		t.Fatalf("results = %#v", results)
	}
}

func TestAnswerColumnsAndFormatNumber(t *testing.T) {
	cases := []struct {
		name       string
		answer     sdk.Answer
		value      string
		confidence string
	}{
		{"noul", sdk.NoulAnswer{Noul: 0.9731}, "0.9731", ""},
		{"choice", sdk.ChoiceAnswer{Choice: "api", Confidence: 0.8}, "api", "0.8000"},
		{"score", sdk.ScoreAnswer{Score: 1.25, Confidence: 0.9}, "1.2500", "0.9000"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value, confidence := AnswerColumns(test.answer)
			if value != test.value || confidence != test.confidence {
				t.Fatalf("columns = %q, %q", value, confidence)
			}
		})
	}
	if got := FormatNumber(-1.2); got != "-1.2000" {
		t.Fatalf("FormatNumber = %q", got)
	}
}
