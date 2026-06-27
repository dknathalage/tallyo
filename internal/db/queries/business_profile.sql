-- name: GetBusinessProfile :one
SELECT * FROM business_profile WHERE tenant_id = $1;

-- name: UpsertBusinessProfile :exec
INSERT INTO business_profile (
    tenant_id, id, name, abn, email, phone, address, logo, metadata, default_currency, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT(tenant_id) DO UPDATE SET
    name = excluded.name,
    abn = excluded.abn,
    email = excluded.email,
    phone = excluded.phone,
    address = excluded.address,
    logo = excluded.logo,
    metadata = excluded.metadata,
    default_currency = excluded.default_currency,
    updated_at = excluded.updated_at;
