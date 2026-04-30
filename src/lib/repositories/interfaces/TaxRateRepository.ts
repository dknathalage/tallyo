import type { TaxRate } from '$lib/types/index.js';

export interface TaxRateRepository {
	getTaxRates(): Promise<TaxRate[]>;
	getDefaultTaxRate(): Promise<TaxRate | null>;
	getTaxRate(id: number): Promise<TaxRate | null>;
	createTaxRate(data: { name: string; rate: number; is_default?: boolean }): Promise<number>;
	updateTaxRate(id: number, data: { name: string; rate: number; is_default?: boolean }): Promise<void>;
	deleteTaxRate(id: number): Promise<void>;
}
