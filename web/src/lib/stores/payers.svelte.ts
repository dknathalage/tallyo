import { createCollectionStore } from './collection.svelte';
import type { Payer, PayerInput } from '$lib/api/types';

export const payers = createCollectionStore<Payer, PayerInput>(
	'payers',
	'payer'
);
