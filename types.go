package gojev

import (
	"encoding/json"
)

// JSONContent represents text, a JSON object, or an array.
type JSONContent = any

// NoulCriteria clarifies what counts as a yes or no answer.
type NoulCriteria struct {
	True  JSONContent `json:"true,omitempty"`
	False JSONContent `json:"false,omitempty"`
}

// NoulQuestion represents a yes/no question.
type NoulQuestion struct {
	Type         string        `json:"type"` // always "noul"
	Instructions JSONContent   `json:"instructions,omitempty"`
	Criteria     *NoulCriteria `json:"criteria,omitempty"`
}

// Noul creates a yes/no question with optional criteria.
func Noul(instructions JSONContent, criteria ...*NoulCriteria) NoulQuestion {
	q := NoulQuestion{
		Type:         "noul",
		Instructions: instructions,
	}
	if len(criteria) > 0 && criteria[0] != nil {
		q.Criteria = criteria[0]
	}
	return q
}

// ChoiceQuestion represents a choice between named alternatives.
type ChoiceQuestion struct {
	Type         string                 `json:"type"` // always "choice"
	Instructions JSONContent            `json:"instructions,omitempty"`
	Criteria     map[string]JSONContent `json:"criteria"`
}

// Choice creates a multiple-choice question with named alternatives.
func Choice(instructions JSONContent, criteria map[string]JSONContent) ChoiceQuestion {
	return ChoiceQuestion{
		Type:         "choice",
		Instructions: instructions,
		Criteria:     criteria,
	}
}

// ScoreQuestion represents a score using an ordered rubric.
type ScoreQuestion struct {
	Type         string        `json:"type"` // always "score"
	Instructions JSONContent   `json:"instructions,omitempty"`
	Criteria     []JSONContent `json:"criteria"`
}

// Score creates a score question using an ordered rubric.
func Score(instructions JSONContent, criteria []JSONContent) ScoreQuestion {
	return ScoreQuestion{
		Type:         "score",
		Instructions: instructions,
		Criteria:     criteria,
	}
}

// Question is an interface implemented by NoulQuestion, ChoiceQuestion, and ScoreQuestion.
type Question interface {
	isQuestion()
}

func (q NoulQuestion) isQuestion()   {}
func (q ChoiceQuestion) isQuestion() {}
func (q ScoreQuestion) isQuestion()  {}

// NoulAnswer is a yes/no answer.
type NoulAnswer struct {
	Type string  `json:"type"` // always "noul"
	Noul float64 `json:"noul"`
}

// ChoiceAnswer is a selected label and its probabilities.
type ChoiceAnswer struct {
	Type          string             `json:"type"` // always "choice"
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// ScoreAnswer is an expected score with its rubric and probabilities.
type ScoreAnswer struct {
	Type          string                 `json:"type"` // always "score"
	Score         float64                `json:"score"`
	Confidence    float64                `json:"confidence"`
	Legend        map[string]JSONContent `json:"legend"`
	Probabilities map[string]float64     `json:"probabilities"`
}

// Answer is a wrapper struct to unmarshal different types of answers.
type Answer struct {
	Type string

	// Only one of these will be non-nil based on Type
	Noul   *NoulAnswer
	Choice *ChoiceAnswer
	Score  *ScoreAnswer
}

// UnmarshalJSON unmarshals an Answer from JSON into the appropriate type (Noul, Choice, or Score).
func (a *Answer) UnmarshalJSON(data []byte) error {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}

	a.Type = header.Type

	switch a.Type {
	case "noul":
		a.Noul = &NoulAnswer{}
		return json.Unmarshal(data, a.Noul)
	case "choice":
		a.Choice = &ChoiceAnswer{}
		return json.Unmarshal(data, a.Choice)
	case "score":
		a.Score = &ScoreAnswer{}
		return json.Unmarshal(data, a.Score)
	default:
		// Unknown type, leave inner pointers nil
		return nil
	}
}

// Usage represents token usage.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// SystemOneRequest is the request payload for the System One API.
type SystemOneRequest struct {
	State     JSONContent         `json:"state"`
	Model     string              `json:"model,omitempty"`
	Questions map[string]Question `json:"questions"`
}

// SystemOneResponse is the response payload from the System One API.
type SystemOneResponse struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Nouls returns all yes/no answers.
func (s *SystemOneResponse) Nouls() map[string]*NoulAnswer {
	m := make(map[string]*NoulAnswer)
	for name, ans := range s.Answers {
		if ans.Noul != nil {
			m[name] = ans.Noul
		}
	}
	return m
}

// Choices returns all choice answers.
func (s *SystemOneResponse) Choices() map[string]*ChoiceAnswer {
	m := make(map[string]*ChoiceAnswer)
	for name, ans := range s.Answers {
		if ans.Choice != nil {
			m[name] = ans.Choice
		}
	}
	return m
}

// Scores returns all score answers.
func (s *SystemOneResponse) Scores() map[string]*ScoreAnswer {
	m := make(map[string]*ScoreAnswer)
	for name, ans := range s.Answers {
		if ans.Score != nil {
			m[name] = ans.Score
		}
	}
	return m
}

// Noul returns a NoulAnswer for a given question name, or nil if not found/wrong type.
func (s *SystemOneResponse) Noul(name string) *NoulAnswer {
	if a, ok := s.Answers[name]; ok && a.Noul != nil {
		return a.Noul
	}
	return nil
}

// Choice returns a ChoiceAnswer for a given question name, or nil if not found/wrong type.
func (s *SystemOneResponse) Choice(name string) *ChoiceAnswer {
	if a, ok := s.Answers[name]; ok && a.Choice != nil {
		return a.Choice
	}
	return nil
}

// Score returns a ScoreAnswer for a given question name, or nil if not found/wrong type.
func (s *SystemOneResponse) Score(name string) *ScoreAnswer {
	if a, ok := s.Answers[name]; ok && a.Score != nil {
		return a.Score
	}
	return nil
}

// ModelMetadata represents metadata for an available model.
type ModelMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
}

// ListModelsResponse is the response payload for listing models.
type ListModelsResponse struct {
	Models []ModelMetadata `json:"models"`
}
