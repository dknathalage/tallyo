-- name: GetBusinessProfile :one
SELECT * FROM business_profile WHERE id = 1;

-- name: UpsertBusinessProfile :exec
INSERT INTO business_profile (
    id, uuid, name, email, phone, address, logo, metadata, default_currency, created_at, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    email = excluded.email,
    phone = excluded.phone,
    address = excluded.address,
    logo = excluded.logo,
    metadata = excluded.metadata,
    default_currency = excluded.default_currency,
    updated_at = excluded.updated_at;
