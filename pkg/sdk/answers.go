package sdk

import (
	"encoding/json"
	"fmt"
)

// Answer is a typed answer returned for a System One question.
type Answer interface {
	json.Marshaler
	answerType() string
}

// NoulAnswer is a probability that the answer is yes.
type NoulAnswer struct {
	Type string  `json:"type,omitempty"`
	Noul float64 `json:"noul"`
}

// ChoiceAnswer is a selected label and its probability distribution.
type ChoiceAnswer struct {
	Type          string             `json:"type,omitempty"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// ScoreAnswer is a probability-weighted score and its distribution.
type ScoreAnswer struct {
	Type          string              `json:"type,omitempty"`
	Score         float64             `json:"score"`
	Confidence    float64             `json:"confidence"`
	Legend        map[int]JSONContent `json:"legend"`
	Probabilities map[int]float64     `json:"probabilities"`
}

func (a NoulAnswer) answerType() string   { return NoulType }
func (a ChoiceAnswer) answerType() string { return ChoiceType }
func (a ScoreAnswer) answerType() string  { return ScoreType }

func (a NoulAnswer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string  `json:"type"`
		Noul float64 `json:"noul"`
	}{NoulType, a.Noul})
}

func (a ChoiceAnswer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type          string             `json:"type"`
		Choice        string             `json:"choice"`
		Confidence    float64            `json:"confidence"`
		Probabilities map[string]float64 `json:"probabilities"`
	}{ChoiceType, a.Choice, a.Confidence, a.Probabilities})
}

func (a ScoreAnswer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type          string              `json:"type"`
		Score         float64             `json:"score"`
		Confidence    float64             `json:"confidence"`
		Legend        map[int]JSONContent `json:"legend"`
		Probabilities map[int]float64     `json:"probabilities"`
	}{ScoreType, a.Score, a.Confidence, a.Legend, a.Probabilities})
}

func (a *NoulAnswer) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := requireType(raw, NoulType); err != nil {
		return err
	}
	if value, ok := raw["noul"]; !ok || string(value) == "null" {
		return fmt.Errorf("noul is required")
	}
	type plain NoulAnswer
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	decoded.Type = NoulType
	*a = NoulAnswer(decoded)
	return nil
}

func (a *ChoiceAnswer) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := requireType(raw, ChoiceType); err != nil {
		return err
	}
	for _, field := range []string{"choice", "confidence", "probabilities"} {
		if value, ok := raw[field]; !ok || string(value) == "null" {
			return fmt.Errorf("%s is required", field)
		}
	}
	type plain ChoiceAnswer
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	decoded.Type = ChoiceType
	*a = ChoiceAnswer(decoded)
	return nil
}

func (a *ScoreAnswer) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := requireType(raw, ScoreType); err != nil {
		return err
	}
	for _, field := range []string{"score", "confidence", "legend", "probabilities"} {
		if value, ok := raw[field]; !ok || string(value) == "null" {
			return fmt.Errorf("%s is required", field)
		}
	}
	type plain ScoreAnswer
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	decoded.Type = ScoreType
	*a = ScoreAnswer(decoded)
	return nil
}

func requireType(raw map[string]json.RawMessage, expected string) error {
	value, ok := raw["type"]
	if !ok {
		return fmt.Errorf("type is required")
	}
	var actual string
	if err := json.Unmarshal(value, &actual); err != nil {
		return fmt.Errorf("type must be a string: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("expected answer type %q, got %q", expected, actual)
	}
	return nil
}
