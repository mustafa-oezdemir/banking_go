-- name: CreateUser :one
INSERT INTO users (email, hashed_password, full_name)
VALUES ($1, $2, $3)
RETURNING id, email, full_name, created_at;

-- name: GetUserByEmail :one
SELECT id, email, hashed_password, created_at, full_name FROM users
WHERE email = $1
LIMIT 1;

-- name: GetUserByID :one
SELECT id, email, hashed_password, created_at, full_name FROM users
WHERE id = $1
LIMIT 1;
