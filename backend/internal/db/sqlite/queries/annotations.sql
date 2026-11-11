-- name: ListAnnotations :many
SELECT * FROM annotations WHERE user_id = ? ORDER BY created_at, id;

-- name: CreateAnnotation :one
INSERT INTO annotations (id, user_id, name, code, view_ids, enabled)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateAnnotation :one
UPDATE annotations SET name = ?, code = ?, view_ids = ?, enabled = ?
WHERE user_id = ? AND id = ? RETURNING *;

-- name: DeleteAnnotation :execrows
DELETE FROM annotations WHERE user_id = ? AND id = ?;
