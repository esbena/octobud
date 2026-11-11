-- name: GetView :one
SELECT * FROM views WHERE user_id = ? AND id = ?;

-- name: ListViews :many
SELECT * FROM views WHERE user_id = ? ORDER BY display_order, name;

-- name: CreateView :one
INSERT INTO views (user_id, name, slug, description, is_default, icon, query, display_order, show_in_sidebar, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE(sqlc.narg(show_in_sidebar), 1), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
RETURNING *;

-- name: UpdateView :one
UPDATE views SET
    name = COALESCE(sqlc.narg(name), name),
    slug = COALESCE(sqlc.narg(slug), slug),
    description = COALESCE(sqlc.arg(description), description),
    icon = COALESCE(sqlc.arg(icon), icon),
    query = COALESCE(sqlc.arg(query), query),
    is_default = COALESCE(sqlc.narg(is_default), is_default),
    show_in_sidebar = COALESCE(sqlc.narg(show_in_sidebar), show_in_sidebar)
WHERE user_id = sqlc.arg(user_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteView :execrows
DELETE FROM views WHERE user_id = ? AND id = ?;

-- name: UpdateViewOrder :exec
UPDATE views SET display_order = ? WHERE user_id = ? AND id = ?;
