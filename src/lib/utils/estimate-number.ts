import { query } from '../db/connection.svelte.js';

export function generateEstimateNumber(): string {
	const result = query<{ max_num: number | null }>(
		`SELECT MAX(CAST(SUBSTR(estimate_number, 5) AS INTEGER)) as max_num FROM estimates WHERE estimate_number GLOB 'EST-[0-9]*'`
	);

	const current = result.length > 0 && result[0].max_num != null ? result[0].max_num : 0;
	return `EST-${String(current + 1).padStart(4, '0')}`;
}
