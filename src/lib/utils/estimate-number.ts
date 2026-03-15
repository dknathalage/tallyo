/**
 * Estimate number generator using the database to find the next sequential number.
 * Only import from server-side code (+page.server.ts, +server.ts, query files).
 */
import { query } from '../db/connection.js';

/**
 * Generates the next sequential estimate number in EST-XXXX format.
 * Uses the database to find the current max and increment by 1.
 */
export function generateEstimateNumber(): string {
	const result = query<{ max_num: number | null }>(
		`SELECT MAX(CAST(SUBSTR(estimate_number, 5) AS INTEGER)) as max_num
		 FROM estimates
		 WHERE estimate_number GLOB 'EST-[0-9]*'`
	);
	const current = result.length > 0 && result[0].max_num != null ? result[0].max_num : 0;
	return `EST-${String(current + 1).padStart(4, '0')}`;
}
