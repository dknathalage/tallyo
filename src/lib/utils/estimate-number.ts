import { query } from '../db/connection.svelte.js';

export function generateEstimateNumber(): string {
	const result = query<{ max_num: string | null }>(
		`SELECT MAX(estimate_number) as max_num FROM estimates`
	);

	if (result.length > 0 && result[0].max_num) {
		const current = parseInt(result[0].max_num.replace('EST-', ''), 10);
		return `EST-${String(current + 1).padStart(4, '0')}`;
	}

	return 'EST-0001';
}
