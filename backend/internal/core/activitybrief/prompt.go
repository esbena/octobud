package activitybrief

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type modelSnapshot struct {
	ReaderUsername string          `json:"readerUsername"`
	Author         string          `json:"author,omitempty"`
	Title          string          `json:"title"`
	Since          time.Time       `json:"since"`
	Warnings       []string        `json:"warnings"`
	NewActivity    []modelEvidence `json:"newActivity"`
	OlderContext   []modelEvidence `json:"olderContext"`
}

type modelEvidence struct {
	Reference       string     `json:"reference"`
	Kind            string     `json:"kind"`
	Change          string     `json:"change"`
	At              *time.Time `json:"at,omitempty"`
	Actor           string     `json:"actor,omitempty"`
	Body            string     `json:"body,omitempty"`
	State           string     `json:"state,omitempty"`
	ParentReference string     `json:"parentReference,omitempty"`
	Detail          string     `json:"detail,omitempty"`
	CreatedAt       *time.Time `json:"createdAt,omitempty"`
	UpdatedAt       *time.Time `json:"updatedAt,omitempty"`
	ActorType       string     `json:"actorType,omitempty"`
	RootReference   string     `json:"rootReference,omitempty"`
	ReviewReference string     `json:"reviewReference,omitempty"`
	Outdated        *bool      `json:"outdated,omitempty"`
	Message         string     `json:"message,omitempty"`
}

type evidenceReferences struct {
	byID        map[string]string
	byReference map[string]string
}

func prepareModelSnapshot(snapshot Snapshot) (modelSnapshot, evidenceReferences, error) {
	references := evidenceReferences{
		byID:        make(map[string]string),
		byReference: make(map[string]string),
	}
	register := func(id string) {
		if id == "" || references.byID[id] != "" {
			return
		}
		reference := evidenceReference(len(references.byReference))
		references.byID[id] = reference
		references.byReference[reference] = id
	}
	for _, event := range append(append([]Evidence{}, snapshot.New...), snapshot.Context...) {
		register(event.ID)
	}
	for _, event := range append(append([]Evidence{}, snapshot.New...), snapshot.Context...) {
		register(event.ParentID)
		register(event.RootID)
		register(event.ReviewID)
	}

	toModelEvidence := func(event Evidence) (modelEvidence, error) {
		detail, err := sanitizePromptDetail(event.Detail)
		if err != nil {
			return modelEvidence{}, fmt.Errorf("sanitize detail for evidence %q: %w", event.ID, err)
		}
		return modelEvidence{
			Reference:       references.byID[event.ID],
			Kind:            event.Kind,
			Change:          event.Change,
			At:              event.At,
			Actor:           event.Actor,
			Body:            event.Body,
			State:           event.State,
			ParentReference: references.byID[event.ParentID],
			Detail:          detail,
			CreatedAt:       event.CreatedAt,
			UpdatedAt:       event.UpdatedAt,
			ActorType:       event.ActorType,
			RootReference:   references.byID[event.RootID],
			ReviewReference: references.byID[event.ReviewID],
			Outdated:        event.Outdated,
			Message:         event.Message,
		}, nil
	}
	result := modelSnapshot{
		ReaderUsername: snapshot.ReaderUsername,
		Author:         snapshot.Author,
		Title:          snapshot.Title,
		Since:          snapshot.Since,
		Warnings:       make([]string, 0, len(snapshot.Warnings)),
		NewActivity:    make([]modelEvidence, 0, len(snapshot.New)),
		OlderContext:   make([]modelEvidence, 0, len(snapshot.Context)),
	}
	for _, warning := range snapshot.Warnings {
		for id, reference := range references.byID {
			warning = strings.ReplaceAll(warning, id, reference)
		}
		if snapshot.NotificationID != "" {
			warning = strings.ReplaceAll(warning, snapshot.NotificationID, "notification")
		}
		result.Warnings = append(result.Warnings, warning)
	}
	for _, event := range snapshot.New {
		evidence, err := toModelEvidence(event)
		if err != nil {
			return modelSnapshot{}, references, err
		}
		result.NewActivity = append(result.NewActivity, evidence)
	}
	for _, event := range snapshot.Context {
		evidence, err := toModelEvidence(event)
		if err != nil {
			return modelSnapshot{}, references, err
		}
		result.OlderContext = append(result.OlderContext, evidence)
	}
	return result, references, nil
}

func evidenceReference(index int) string {
	var label []byte
	for index >= 0 {
		label = append([]byte{byte('a' + index%26)}, label...)
		index = index/26 - 1
	}
	return "source-" + string(label)
}

func sanitizePromptDetail(detail string) (string, error) {
	if detail == "" {
		return "", nil
	}
	var value any
	if err := json.Unmarshal([]byte(detail), &value); err != nil {
		return "", fmt.Errorf("decode JSON: %w", err)
	}
	value = stripInternalDetailFields(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode sanitized JSON: %w", err)
	}
	return string(encoded), nil
}

func stripInternalDetailFields(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if strings.HasSuffix(normalized, "id") || strings.HasSuffix(normalized, "url") {
				continue
			}
			result[key] = stripInternalDetailFields(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = stripInternalDetailFields(child)
		}
		return result
	default:
		return value
	}
}

func resolveEvidenceReferences(arguments any, references evidenceReferences) (any, error) {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, errors.New("invalid summary arguments")
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, errors.New("invalid summary arguments")
	}
	if err := resolveEvidenceReferencesInValue(value, references); err != nil {
		return nil, err
	}
	encoded, err = json.Marshal(value)
	if err != nil {
		return nil, errors.New("invalid summary arguments")
	}
	var resolved any
	if err := json.Unmarshal(encoded, &resolved); err != nil {
		return nil, errors.New("invalid summary arguments")
	}
	return resolved, nil
}

func resolveEvidenceReferencesInValue(value any, references evidenceReferences) error {
	switch typed := value.(type) {
	case map[string]any:
		if _, exists := typed["evidenceIds"]; exists {
			return errors.New("use evidenceRefs to cite activity")
		}
		if raw, exists := typed["evidenceRefs"]; exists {
			items, ok := raw.([]any)
			if !ok {
				return errors.New("evidenceRefs must be an array")
			}
			ids := make([]string, len(items))
			for index, item := range items {
				reference, ok := item.(string)
				if !ok {
					return errors.New("evidenceRefs must contain strings")
				}
				id, ok := references.byReference[reference]
				if !ok {
					return errors.New("summary cites an unknown evidence reference")
				}
				ids[index] = id
			}
			typed["evidenceIds"] = ids
			delete(typed, "evidenceRefs")
		}
		for key, child := range typed {
			if key == "evidenceIds" {
				continue
			}
			if err := resolveEvidenceReferencesInValue(child, references); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := resolveEvidenceReferencesInValue(child, references); err != nil {
				return err
			}
		}
	}
	return nil
}

func containsModelReference(text string, references evidenceReferences) bool {
	for reference := range references.byReference {
		if strings.Contains(text, reference) {
			return true
		}
	}
	return false
}

func containsInternalIdentifier(text string, snapshot Snapshot) bool {
	identifiers := []string{snapshot.NotificationID}
	for _, event := range append(append([]Evidence{}, snapshot.New...), snapshot.Context...) {
		identifiers = append(identifiers, event.ID)
	}
	for _, identifier := range identifiers {
		if identifier == "" {
			continue
		}
		if strings.Contains(text, identifier) {
			return true
		}
		suffix := identifier[strings.LastIndex(identifier, ":")+1:]
		if len(suffix) >= 9 && isDigits(suffix) && containsIdentifierToken(text, suffix) {
			return true
		}
	}
	return false
}

func isDigits(value string) bool {
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return value != ""
}

func containsIdentifierToken(text, identifier string) bool {
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], identifier)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(identifier)
		leftBoundary := start == 0 || !isASCIIAlphanumeric(text[start-1])
		rightBoundary := end == len(text) || !isASCIIAlphanumeric(text[end])
		if leftBoundary && rightBoundary {
			return true
		}
		offset = start + 1
	}
	return false
}

func isASCIIAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_'
}
