import { createCollectionStore } from './collection.svelte';
import type { RecurringTemplate, RecurringInput } from '$lib/api/types';

export const recurring = createCollectionStore<RecurringTemplate, RecurringInput>(
	'recurring',
	'recurring_template'
);
