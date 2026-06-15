-- name: GetBusinessProfile :one
SELECT * FROM business_profile WHERE tenant_id = ?;

-- name: UpsertBusinessProfile :exec
INSERT INTO business_profile (
    tenant_id, uuid, name, abn, email, phone, address, zone, logo, metadata, default_currency, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tenant_id) DO UPDATE SET
    name = excluded.name,
    abn = excluded.abn,
    email = excluded.email,
    phone = excluded.phone,
    address = excluded.address,
    zone = excluded.zone,
    logo = excluded.logo,
    metadata = excluded.metadata,
    default_currency = excluded.default_currency,
    updated_at = excluded.updated_at;

-- name: UpdateBusinessZone :exec
UPDATE business_profile SET zone = ?, updated_at = ? WHERE tenant_id = ?;
