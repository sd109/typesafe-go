package sdk

import (
	"encoding/json"
	"fmt"
)

// Question is a discriminated System One question.
type Question interface {
	json.Marshaler
	validateQuestion(name string) error
}

// NoulCriteria describes the true and false outcomes of a noul question.
// A nil value means that an outcome is intentionally undescribed.
type NoulCriteria map[string]JSONContent

// NoulQuestion asks a yes/no question.
type NoulQuestion struct {
	Instructions JSONContent  `json:"instructions,omitempty"`
	Criteria     NoulCriteria `json:"criteria,omitempty"`
}

// ChoiceQuestion selects one label from Criteria.
type ChoiceQuestion struct {
	Instructions JSONContent            `json:"instructions,omitempty"`
	Criteria     map[string]JSONContent `json:"criteria"`
}

// ScoreQuestion rates content against an ordered, zero-based rubric.
type ScoreQuestion struct {
	Instructions JSONContent   `json:"instructions,omitempty"`
	Criteria     []JSONContent `json:"criteria"`
}

func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	if err := q.validateQuestion("question"); err != nil {
		return nil, err
	}
	body := map[string]any{"type": NoulType}
	if q.Instructions != nil {
		body["instructions"] = q.Instructions
	}
	if q.Criteria != nil {
		body["criteria"] = q.Criteria
	}
	return json.Marshal(body)
}

func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	if err := q.validateQuestion("question"); err != nil {
		return nil, err
	}
	body := map[string]any{"type": ChoiceType, "criteria": q.Criteria}
	if q.Instructions != nil {
		body["instructions"] = q.Instructions
	}
	return json.Marshal(body)
}

func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	if err := q.validateQuestion("question"); err != nil {
		return nil, err
	}
	body := map[string]any{"type": ScoreType, "criteria": q.Criteria}
	if q.Instructions != nil {
		body["instructions"] = q.Instructions
	}
	return json.Marshal(body)
}

func (q NoulQuestion) validateQuestion(name string) error {
	if q.Instructions != nil {
		if err := validateContent(q.Instructions, name+".instructions"); err != nil {
			return err
		}
	}
	if q.Criteria == nil {
		return nil
	}
	for key, value := range q.Criteria {
		if key != "true" && key != "false" {
			return fmt.Errorf("question %q has invalid noul criteria key %q", name, key)
		}
		if value != nil {
			if err := validateContent(value, name+".criteria."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (q ChoiceQuestion) validateQuestion(name string) error {
	if q.Instructions != nil {
		if err := validateContent(q.Instructions, name+".instructions"); err != nil {
			return err
		}
	}
	if len(q.Criteria) == 0 {
		return fmt.Errorf("choice question %q requires criteria", name)
	}
	for key, value := range q.Criteria {
		if key == "" {
			return fmt.Errorf("choice question %q has an empty criterion name", name)
		}
		if value != nil {
			if err := validateContent(value, name+".criteria."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (q ScoreQuestion) validateQuestion(name string) error {
	if q.Instructions != nil {
		if err := validateContent(q.Instructions, name+".instructions"); err != nil {
			return err
		}
	}
	if len(q.Criteria) == 0 {
		return fmt.Errorf("score question %q has no criteria; at least one score is required", name)
	}
	for i, value := range q.Criteria {
		if value == nil {
			return fmt.Errorf("%s.criteria[%d] must not be null", name, i)
		}
		if err := validateContent(value, fmt.Sprintf("%s.criteria[%d]", name, i)); err != nil {
			return err
		}
	}
	return nil
}
