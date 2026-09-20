package questioncli

import (
	"testing"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

func TestParseQuestionsPreservesStableOrderAndTypes(t *testing.T) {
	questions, queries, err := ParseQuestions(
		[]string{" first? ", "second?"},
		[]string{"kind? {log, meeting note}"},
		[]string{"severity? [low, high]"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id       string
		kind     Kind
		question string
	}{
		{"noul-1", KindNoul, "first?"},
		{"noul-2", KindNoul, "second?"},
		{"choice-1", KindChoice, "kind? {log, meeting note}"},
		{"score-1", KindScore, "severity? [low, high]"},
	}
	if len(queries) != len(want) {
		t.Fatalf("query count = %d, want %d", len(queries), len(want))
	}
	for index, expected := range want {
		if queries[index] != (Query{ID: expected.id, Kind: expected.kind, Question: expected.question}) {
			t.Fatalf("query[%d] = %#v, want %#v", index, queries[index], expected)
		}
	}
	if _, ok := questions["noul-1"].(sdk.NoulQuestion); !ok {
		t.Fatalf("noul type = %T", questions["noul-1"])
	}
	choice, ok := questions["choice-1"].(sdk.ChoiceQuestion)
	if !ok || len(choice.Criteria) != 2 || choice.Criteria["meeting note"] != nil {
		t.Fatalf("choice = %#v", questions["choice-1"])
	}
	score, ok := questions["score-1"].(sdk.ScoreQuestion)
	if !ok || len(score.Criteria) != 2 || score.Criteria[1] != "high" {
		t.Fatalf("score = %#v", questions["score-1"])
	}
}

func TestParseQuestionsRequiresAtLeastOneQuestion(t *testing.T) {
	if _, _, err := ParseQuestions(nil, nil, nil); err == nil {
		t.Fatal("expected missing question error")
	}
}

func TestParseQuestionRejectsMalformedCriteria(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) (sdk.Question, error)
		text string
	}{
		{"empty noul", ParseNoul, " "},
		{"missing choice group", ParseChoice, "what is it?"},
		{"multiple choice groups", ParseChoice, "what {is} {it}?"},
		{"nested choice group", ParseChoice, "what {{is, it}}?"},
		{"duplicate choice criterion", ParseChoice, "what {a, a}?"},
		{"empty score criterion", ParseScore, "how old? [day, , year]"},
		{"mismatched score delimiter", ParseScore, "how old? {day, week}"},
		{"square brackets in choice", ParseChoice, "what {a, b} [extra]?"},
		{"curly braces in score", ParseScore, "what {a, b} [low, high]?"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.fn(test.text); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
