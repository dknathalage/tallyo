# components/import/

Multi-step import wizard for CSV/Excel files.

- `ImportWizardModal.svelte` — Main wizard modal orchestrating the steps
- `StepFileSelect.svelte` — Step 1: File picker
- `StepColumnMapping.svelte` — Step 2: Map source columns to app fields
- `StepImportMode.svelte` — Step 3: Choose merge/replace strategy
- `StepPreviewDiff.svelte` — Step 4: Preview changes before committing
