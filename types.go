package gojev

import (
	"encoding/base64"
	"encoding/json"
)

// JSONContent represents text, a JSON object, or an array.
type JSONContent = any

// Question and answer type identifiers used on the wire.
const (
	TypeNoul   = "noul"
	TypeChoice = "choice"
	TypeScore  = "score"
	// TypeBox locates a region in an image. It requires Jev 1.14 and the
	// TypeSafe-Danger-Zone-Images header (see WithDangerZoneImages).
	TypeBox = "box"
)

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
		Type:         TypeNoul,
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
		Type:         TypeChoice,
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
		Type:         TypeScore,
		Instructions: instructions,
		Criteria:     criteria,
	}
}

// BoxQuestion asks for the bounding box of something in an image asset.
type BoxQuestion struct {
	Type         string      `json:"type"` // always "box"
	Instructions JSONContent `json:"instructions,omitempty"`
}

// Box creates a box question. It requires Jev 1.14 and the
// TypeSafe-Danger-Zone-Images header (see WithDangerZoneImages).
func Box(instructions JSONContent) BoxQuestion {
	return BoxQuestion{Type: TypeBox, Instructions: instructions}
}

// Question is an interface implemented by NoulQuestion, ChoiceQuestion, ScoreQuestion, and BoxQuestion.
type Question interface {
	isQuestion()
}

// MarshalJSON always encodes the question with type "noul".
func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	type plain NoulQuestion
	p := plain(q)
	p.Type = TypeNoul
	return json.Marshal(p)
}

// MarshalJSON always encodes the question with type "choice".
func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	type plain ChoiceQuestion
	p := plain(q)
	p.Type = TypeChoice
	return json.Marshal(p)
}

// MarshalJSON always encodes the question with type "score".
func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	type plain ScoreQuestion
	p := plain(q)
	p.Type = TypeScore
	return json.Marshal(p)
}

// MarshalJSON always encodes the question with type "box".
func (q BoxQuestion) MarshalJSON() ([]byte, error) {
	type plain BoxQuestion
	p := plain(q)
	p.Type = TypeBox
	return json.Marshal(p)
}

func (q NoulQuestion) isQuestion()   {}
func (q ChoiceQuestion) isQuestion() {}
func (q ScoreQuestion) isQuestion()  {}
func (q BoxQuestion) isQuestion()    {}

// Asset is an image referenced from state, instructions or criteria by its
// placeholder key in SystemOneRequest.Assets. Requests with assets need the
// TypeSafe-Danger-Zone-Images header (see WithDangerZoneImages).
type Asset struct {
	Type       string `json:"type"`   // always "image"
	Format     string `json:"format"` // "png", "jpg" or "jpeg"
	DataBase64 string `json:"data_base64"`
}

// Image creates an image asset from raw bytes. format is "png", "jpg" or "jpeg".
func Image(format string, data []byte) Asset {
	return Asset{Type: "image", Format: format, DataBase64: base64.StdEncoding.EncodeToString(data)}
}

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

// BoxRegion is a bounding box with edges as fractions of the image from 0 to 1.
// Top and bottom are measured from the top edge, left and right from the left edge.
type BoxRegion struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
}

// BoxAnswer is the located region; Confidence may be nil.
type BoxAnswer struct {
	Type       string    `json:"type"` // always "box"
	Box        BoxRegion `json:"box"`
	Confidence *float64  `json:"confidence"`
}

// Answer is a wrapper struct to unmarshal different types of answers.
type Answer struct {
	Type string

	// Only one of these will be non-nil based on Type
	Noul   *NoulAnswer
	Choice *ChoiceAnswer
	Score  *ScoreAnswer
	Box    *BoxAnswer

	// raw keeps the original JSON of an answer with an unknown Type so it
	// survives a marshal round trip. It is a string so Answer stays comparable.
	raw string
}

// UnmarshalJSON unmarshals an Answer from JSON into the appropriate type (Noul, Choice, or Score).
func (a *Answer) UnmarshalJSON(data []byte) error {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}

	*a = Answer{Type: header.Type}

	switch a.Type {
	case TypeNoul:
		a.Noul = &NoulAnswer{}
		return json.Unmarshal(data, a.Noul)
	case TypeChoice:
		a.Choice = &ChoiceAnswer{}
		return json.Unmarshal(data, a.Choice)
	case TypeScore:
		a.Score = &ScoreAnswer{}
		return json.Unmarshal(data, a.Score)
	case TypeBox:
		a.Box = &BoxAnswer{}
		return json.Unmarshal(data, a.Box)
	default:
		// Unknown type: leave inner pointers nil but keep the payload.
		a.raw = string(data)
		return nil
	}
}

// MarshalJSON encodes an Answer in the same flat wire format UnmarshalJSON reads.
// The type field is always derived from which answer is set.
func (a Answer) MarshalJSON() ([]byte, error) {
	switch {
	case a.Noul != nil:
		n := *a.Noul
		n.Type = TypeNoul
		return json.Marshal(n)
	case a.Choice != nil:
		c := *a.Choice
		c.Type = TypeChoice
		return json.Marshal(c)
	case a.Score != nil:
		s := *a.Score
		s.Type = TypeScore
		return json.Marshal(s)
	case a.Box != nil:
		b := *a.Box
		b.Type = TypeBox
		return json.Marshal(b)
	case a.raw != "":
		return []byte(a.raw), nil
	default:
		return json.Marshal(struct {
			Type string `json:"type"`
		}{a.Type})
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
	// Assets maps placeholder keys used in State, instructions or criteria to images.
	Assets map[string]Asset `json:"assets,omitempty"`
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

// Boxes returns all box answers.
func (s *SystemOneResponse) Boxes() map[string]*BoxAnswer {
	m := make(map[string]*BoxAnswer)
	for name, ans := range s.Answers {
		if ans.Box != nil {
			m[name] = ans.Box
		}
	}
	return m
}

// Box returns a BoxAnswer for a given question name, or nil if not found/wrong type.
func (s *SystemOneResponse) Box(name string) *BoxAnswer {
	if a, ok := s.Answers[name]; ok && a.Box != nil {
		return a.Box
	}
	return nil
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
