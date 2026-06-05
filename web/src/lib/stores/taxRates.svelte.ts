import { createCollectionStore } from './collection.svelte';
import type { TaxRate, TaxRateInput } from '$lib/api/types';

export const taxRates = createCollectionStore<TaxRate, TaxRateInput>('tax-rates', 'tax_rate');
