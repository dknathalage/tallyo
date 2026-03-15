import { execute, query, save } from '../connection.js';
import type { TaxRate } from '../../types/index.js';

export function getTaxRates(): TaxRate[] {
	return query<TaxRate>(`SELECT * FROM tax_rates ORDER BY is_default DESC, name ASC`);
}

export function getDefaultTaxRate(): TaxRate | null {
	const results = query<TaxRate>(`SELECT * FROM tax_rates WHERE is_default = 1 LIMIT 1`);
	return results.length > 0 ? results[0] : null;
}

export function getTaxRate(id: number): TaxRate | null {
	const results = query<TaxRate>(`SELECT * FROM tax_rates WHERE id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

export function createTaxRate(data: { name: string; rate: number; is_default?: boolean }): number {
	if (data.is_default) {
		execute(`UPDATE tax_rates SET is_default = 0`);
	}
	execute(
		`INSERT INTO tax_rates (uuid, name, rate, is_default) VALUES (?, ?, ?, ?)`,
		[crypto.randomUUID(), data.name, data.rate, data.is_default ? 1 : 0]
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	save();
	return result[0].id;
}

export function updateTaxRate(
	id: number,
	data: { name: string; rate: number; is_default?: boolean }
): void {
	if (data.is_default) {
		execute(`UPDATE tax_rates SET is_default = 0 WHERE id != ?`, [id]);
	}
	execute(
		`UPDATE tax_rates SET name = ?, rate = ?, is_default = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.rate, data.is_default ? 1 : 0, id]
	);
	save();
}

export function deleteTaxRate(id: number): void {
	execute(`DELETE FROM tax_rates WHERE id = ?`, [id]);
	save();
}
