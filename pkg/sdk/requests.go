package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// SystemOneRequest is the request body for POST /v1/systemone.
type SystemOneRequest struct {
	State     JSONContent         `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// Validate checks the request against the public API contract.
func (r SystemOneRequest) Validate() error {
	if err := validateContent(r.State, "state"); err != nil {
		return err
	}
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Questions) == 0 {
		return fmt.Errorf("at least one question is required")
	}
	for name, question := range r.Questions {
		if name == "" {
			return fmt.Errorf("question name must not be empty")
		}
		if question == nil {
			return fmt.Errorf("question %q must not be nil", name)
		}
		if err := question.validateQuestion(name); err != nil {
			return err
		}
	}
	return nil
}

func (r SystemOneRequest) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	type wireRequest struct {
		State     JSONContent         `json:"state"`
		Model     string              `json:"model"`
		Questions map[string]Question `json:"questions"`
	}
	return json.Marshal(wireRequest{r.State, r.Model, r.Questions})
}

// UnmarshalJSON decodes known question discriminators into their typed forms.
func (r *SystemOneRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		State     json.RawMessage            `json:"state"`
		Model     string                     `json:"model"`
		Questions map[string]json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.State) == 0 {
		return fmt.Errorf("state is required")
	}
	var state any
	if err := json.Unmarshal(raw.State, &state); err != nil {
		return err
	}
	questions := make(map[string]Question, len(raw.Questions))
	for name, payload := range raw.Questions {
		question, err := decodeQuestion(payload)
		if err != nil {
			return fmt.Errorf("question %q: %w", name, err)
		}
		questions[name] = question
	}
	r.State, r.Model, r.Questions = state, raw.Model, questions
	return nil
}

func decodeQuestion(data []byte) (Question, error) {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}
	if tag.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	var question Question
	switch tag.Type {
	case NoulType:
		question = new(NoulQuestion)
	case ChoiceType:
		question = new(ChoiceQuestion)
	case ScoreType:
		question = new(ScoreQuestion)
	default:
		return nil, fmt.Errorf("unsupported question type %q", tag.Type)
	}
	if err := json.Unmarshal(data, question); err != nil {
		return nil, err
	}
	return question, nil
}

func mergeRequestBody(request SystemOneRequest, extra map[string]any) ([]byte, error) {
	base, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var body map[string]any
	if err := json.Unmarshal(base, &body); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if err := validateJSON(value, "extra_body."+key, false); err != nil {
			return nil, err
		}
		body[key] = value
	}
	return json.Marshal(body)
}

func compactJSON(data []byte) string {
	var out bytes.Buffer
	if json.Compact(&out, data) == nil {
		return out.String()
	}
	return string(data)
}
