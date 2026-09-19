package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Usage contains token counts reported by the API. A zero value also represents
// an omitted count, as the API may return an empty usage object.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// SystemOneResponse is the response from POST /v1/systemone.
type SystemOneResponse struct {
	Model    string            `json:"model"`
	Answers  map[string]Answer `json:"answers"`
	Usage    Usage             `json:"usage"`
	Metadata ResponseMetadata  `json:"-"`
}

// ModelMetadata describes a model available to the account.
type ModelMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
}

// ListModelsResponse is the response from GET /v1/models.
type ListModelsResponse struct {
	Models   []ModelMetadata  `json:"models"`
	Metadata ResponseMetadata `json:"-"`
}

// ValidationError is one entry in a FastAPI/Pydantic validation response.
type ValidationError struct {
	Loc   []any          `json:"loc"`
	Msg   string         `json:"msg"`
	Type  string         `json:"type"`
	Input any            `json:"input,omitempty"`
	Ctx   map[string]any `json:"ctx,omitempty"`
}

// HTTPValidationError is the common 422 response body.
type HTTPValidationError struct {
	Detail []ValidationError `json:"detail,omitempty"`
}

// ResponseMetadata contains HTTP information associated with an SDK response.
type ResponseMetadata struct {
	StatusCode int
	Headers    http.Header
	RequestID  string
	RawBody    []byte
}

func (m ResponseMetadata) clone() ResponseMetadata {
	return ResponseMetadata{
		StatusCode: m.StatusCode,
		Headers:    cloneHeader(m.Headers),
		RequestID:  m.RequestID,
		RawBody:    append([]byte(nil), m.RawBody...),
	}
}

func (r *SystemOneResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Model   *string         `json:"model"`
		Answers json.RawMessage `json:"answers"`
		Usage   *Usage          `json:"usage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Model == nil {
		return &ResponseValidationError{FieldPath: "model", Body: json.RawMessage(data)}
	}
	if raw.Usage == nil {
		return &ResponseValidationError{FieldPath: "usage", Body: json.RawMessage(data)}
	}
	if len(raw.Answers) == 0 || string(raw.Answers) == "null" {
		return &ResponseValidationError{FieldPath: "answers", Body: json.RawMessage(data)}
	}
	var answerRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw.Answers, &answerRaw); err != nil {
		return &ResponseValidationError{FieldPath: "answers", Body: json.RawMessage(data)}
	}
	answers := make(map[string]Answer, len(answerRaw))
	for name, payload := range answerRaw {
		answer, known, err := decodeAnswer(payload)
		if err != nil {
			return &ResponseValidationError{FieldPath: "answers." + name + "." + errorField(err), Body: json.RawMessage(data), Cause: err}
		}
		if known {
			answers[name] = answer
		}
	}
	r.Model, r.Usage, r.Answers = *raw.Model, *raw.Usage, answers
	return nil
}

func decodeAnswer(data []byte) (Answer, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return nil, true, fmt.Errorf("type is required")
	}
	typeValue, ok := raw["type"]
	if !ok {
		return nil, true, fmt.Errorf("type is required")
	}
	var tag string
	if err := json.Unmarshal(typeValue, &tag); err != nil {
		return nil, true, fmt.Errorf("type must be a string")
	}
	switch tag {
	case NoulType:
		var answer NoulAnswer
		if err := json.Unmarshal(data, &answer); err != nil {
			return nil, true, err
		}
		return answer, true, nil
	case ChoiceType:
		var answer ChoiceAnswer
		if err := json.Unmarshal(data, &answer); err != nil {
			return nil, true, err
		}
		return answer, true, nil
	case ScoreType:
		var answer ScoreAnswer
		if err := json.Unmarshal(data, &answer); err != nil {
			return nil, true, err
		}
		return answer, true, nil
	default:
		return nil, false, nil
	}
}

func errorField(err error) string {
	message := err.Error()
	for _, field := range []string{"noul", "choice", "confidence", "probabilities", "score", "legend", "type"} {
		if len(message) >= len(field) && containsWord(message, field) {
			return field
		}
	}
	return "value"
}

func containsWord(s, word string) bool {
	for i := 0; i+len(word) <= len(s); i++ {
		if s[i:i+len(word)] != word {
			continue
		}
		before, after := i == 0 || s[i-1] < 'a' || s[i-1] > 'z', i+len(word) == len(s) || s[i+len(word)] < 'a' || s[i+len(word)] > 'z'
		if before && after {
			return true
		}
	}
	return false
}

func (r *ListModelsResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Models json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Models) == 0 || string(raw.Models) == "null" {
		return &ResponseValidationError{FieldPath: "models", Body: json.RawMessage(data)}
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Models, &entries); err != nil {
		return &ResponseValidationError{FieldPath: "models", Body: json.RawMessage(data), Cause: err}
	}
	models := make([]ModelMetadata, len(entries))
	for i, entry := range entries {
		for _, field := range []string{"name", "description", "release_date"} {
			value, ok := entry[field]
			if !ok || string(value) == "null" {
				return &ResponseValidationError{FieldPath: fmt.Sprintf("models[%d].%s", i, field), Body: json.RawMessage(data)}
			}
		}
		if err := json.Unmarshal(mustRaw(entry["name"]), &models[i].Name); err != nil {
			return &ResponseValidationError{FieldPath: fmt.Sprintf("models[%d].name", i), Body: json.RawMessage(data), Cause: err}
		}
		if err := json.Unmarshal(mustRaw(entry["description"]), &models[i].Description); err != nil {
			return &ResponseValidationError{FieldPath: fmt.Sprintf("models[%d].description", i), Body: json.RawMessage(data), Cause: err}
		}
		if err := json.Unmarshal(mustRaw(entry["release_date"]), &models[i].ReleaseDate); err != nil {
			return &ResponseValidationError{FieldPath: fmt.Sprintf("models[%d].release_date", i), Body: json.RawMessage(data), Cause: err}
		}
	}
	r.Models = models
	return nil
}

func mustRaw(value json.RawMessage) []byte { return value }

func cloneHeader(header http.Header) http.Header {
	if header == nil {
		return nil
	}
	return header.Clone()
}
