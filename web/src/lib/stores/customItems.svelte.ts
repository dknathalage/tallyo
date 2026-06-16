import { createCollectionStore } from './collection.svelte';
import type { CustomItem, CustomItemInput } from '$lib/api/types';

export const customItems = createCollectionStore<CustomItem, CustomItemInput>(
	'custom-items',
	'custom_item'
);
