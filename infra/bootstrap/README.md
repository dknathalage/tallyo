# Bootstrap (one-time, local state)

This tiny tofu root solves the chicken-and-egg of remote state: it creates the
GCS bucket that backs every other unit's state, and enables the core GCP APIs.
It uses **local state** (no backend) and is meant to be run once, by a human,
with real credentials and active billing.

## Prerequisites

- A GCP project with **billing enabled**.
- `gcloud auth application-default login`
- `gcloud config set project <PROJECT_ID>`
- `tofu` >= 1.8

## Run (one-time human step)

```bash
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars: set project_id, region, and a globally-unique state_bucket_name
tofu init
tofu apply
```

Record the `state_bucket_name` output — it MUST match `state_bucket` in
`infra/live/terragrunt.hcl`.

## Credential-free verification

```bash
tofu fmt -check
tofu init -backend=false
tofu validate
```
