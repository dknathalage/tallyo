import {
	getTaxRates,
	getDefaultTaxRate,
	getTaxRate,
	createTaxRate,
	updateTaxRate,
	deleteTaxRate
} from '$lib/db/queries/tax-rates.js';
import type { TaxRateRepository } from '../interfaces/TaxRateRepository.js';
import type { TaxRate } from '$lib/types/index.js';

export class PgTaxRateRepository implements TaxRateRepository {
	async getTaxRates(): Promise<TaxRate[]> {
		return await getTaxRates();
	}

	async getDefaultTaxRate(): Promise<TaxRate | null> {
		return await getDefaultTaxRate();
	}

	async getTaxRate(id: number): Promise<TaxRate | null> {
		return await getTaxRate(id);
	}

	async createTaxRate(data: { name: string; rate: number; is_default?: boolean }): Promise<number> {
		return await createTaxRate(data);
	}

	async updateTaxRate(id: number, data: { name: string; rate: number; is_default?: boolean }): Promise<void> {
		return await updateTaxRate(id, data);
	}

	async deleteTaxRate(id: number): Promise<void> {
		return await deleteTaxRate(id);
	}
}
