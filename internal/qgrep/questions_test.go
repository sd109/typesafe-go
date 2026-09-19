package qgrep

import (
	"testing"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

func TestParseQuestions(t *testing.T) {
	questions, queries, err := parseQuestions(
		[]string{"Does it contain PII?"},
		[]string{"What is it? {log, meeting note, other}"},
		[]string{"How old is it? [day, week, year]"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(queries), 3; got != want {
		t.Fatalf("query count = %d, want %d", got, want)
	}
	for index, want := range []struct {
		id   string
		kind questionKind
	}{
		{"noul-1", kindNoul},
		{"choice-1", kindChoice},
		{"score-1", kindScore},
	} {
		if queries[index].id != want.id || queries[index].kind != want.kind {
			t.Fatalf("query[%d] = %#v, want id=%q kind=%q", index, queries[index], want.id, want.kind)
		}
	}
	choice, ok := questions["choice-1"].(sdk.ChoiceQuestion)
	if !ok {
		t.Fatalf("choice type = %T", questions["choice-1"])
	}
	if len(choice.Criteria) != 3 || choice.Criteria["meeting note"] != nil {
		t.Fatalf("choice criteria = %#v", choice.Criteria)
	}
	score, ok := questions["score-1"].(sdk.ScoreQuestion)
	if !ok || len(score.Criteria) != 3 || score.Criteria[1] != "week" {
		t.Fatalf("score criteria = %#v", questions["score-1"])
	}
}

func TestParseQuestionsRejectsMalformedCriteria(t *testing.T) {
	cases := []struct {
		name  string
		fn    func(string) (sdk.Question, error)
		value string
	}{
		{"missing choice group", parseChoice, "what is it?"},
		{"multiple choice groups", parseChoice, "what {is} {it}?"},
		{"nested choice group", parseChoice, "what {{is, it}}?"},
		{"duplicate choice criterion", parseChoice, "what {a, a}?"},
		{"empty score criterion", parseScore, "how old? [day, , year]"},
		{"mismatched score delimiter", parseScore, "how old? {day, week}"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.fn(test.value); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
