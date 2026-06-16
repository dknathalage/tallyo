import { createCollectionStore } from './collection.svelte';
import type { PlanManager, PlanManagerInput } from '$lib/api/types';

export const planManagers = createCollectionStore<PlanManager, PlanManagerInput>(
	'plan-managers',
	'plan_manager'
);
