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

export class SqliteTaxRateRepository implements TaxRateRepository {
	getTaxRates(): TaxRate[] {
		return getTaxRates();
	}

	getDefaultTaxRate(): TaxRate | null {
		return getDefaultTaxRate();
	}

	getTaxRate(id: number): TaxRate | null {
		return getTaxRate(id);
	}

	createTaxRate(data: { name: string; rate: number; is_default?: boolean }): Promise<number> {
		return createTaxRate(data);
	}

	updateTaxRate(id: number, data: { name: string; rate: number; is_default?: boolean }): Promise<void> {
		return updateTaxRate(id, data);
	}

	deleteTaxRate(id: number): Promise<void> {
		return deleteTaxRate(id);
	}
}
