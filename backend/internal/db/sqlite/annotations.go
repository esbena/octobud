package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/octobud-hq/octobud/backend/internal/db"
)

func decodeAnnotation(row Annotation) (db.Annotation, error) {
	result := db.Annotation{ID: row.ID, Name: row.Name, Code: row.Code, ViewIDs: []string{}, Enabled: row.Enabled != 0}
	if err := json.Unmarshal([]byte(row.ViewIds), &result.ViewIDs); err != nil {
		return db.Annotation{}, err
	}
	return result, nil
}

func (s *Store) ListAnnotations(ctx context.Context, userID string) ([]db.Annotation, error) {
	rows, err := db.RetryOnBusy(ctx, func() ([]Annotation, error) {
		return s.q.ListAnnotations(ctx, userID)
	})
	if err != nil {
		return nil, err
	}
	result := make([]db.Annotation, 0, len(rows))
	for _, row := range rows {
		item, err := decodeAnnotation(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Store) CreateAnnotation(ctx context.Context, userID string, annotation db.Annotation) (db.Annotation, error) {
	viewIDs, err := json.Marshal(annotation.ViewIDs)
	if err != nil {
		return db.Annotation{}, err
	}
	row, err := db.RetryOnBusy(ctx, func() (Annotation, error) {
		return s.q.CreateAnnotation(ctx, CreateAnnotationParams{
			ID: annotation.ID, UserID: userID, Name: annotation.Name,
			Code: annotation.Code, ViewIds: string(viewIDs), Enabled: boolToInt64(annotation.Enabled),
		})
	})
	if err != nil {
		return db.Annotation{}, err
	}
	return decodeAnnotation(row)
}

func (s *Store) UpdateAnnotation(ctx context.Context, userID string, annotation db.Annotation) (db.Annotation, error) {
	viewIDs, err := json.Marshal(annotation.ViewIDs)
	if err != nil {
		return db.Annotation{}, err
	}
	row, err := db.RetryOnBusy(ctx, func() (Annotation, error) {
		return s.q.UpdateAnnotation(ctx, UpdateAnnotationParams{
			ID: annotation.ID, UserID: userID, Name: annotation.Name,
			Code: annotation.Code, ViewIds: string(viewIDs), Enabled: boolToInt64(annotation.Enabled),
		})
	})
	if err != nil {
		return db.Annotation{}, err
	}

	return decodeAnnotation(row)
}

func (s *Store) DeleteAnnotation(ctx context.Context, userID, id string) error {
	count, err := db.RetryOnBusy(ctx, func() (int64, error) {
		return s.q.DeleteAnnotation(ctx, DeleteAnnotationParams{UserID: userID, ID: id})
	})
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
