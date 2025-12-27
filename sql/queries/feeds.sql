-- name: CreateFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetFeeds :many
SELECT *
FROM feeds
ORDER BY created_at DESC;

-- name: GetNextFeedsToFetch :many
SELECT *
FROM feeds
ORDER BY last_fetched_at IS NOT NULL, last_fetched_at, created_at
LIMIT $1;

-- name: MarkFeedFetched :exec
UPDATE feeds
SET last_fetched_at = NOW(), updated_at = NOW()
WHERE id = $1;
