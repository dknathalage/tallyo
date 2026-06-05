import { createCollectionStore } from './collection.svelte';
import type { CatalogItem, CatalogItemInput } from '$lib/api/types';

export const catalog = createCollectionStore<CatalogItem, CatalogItemInput>('catalog', 'catalog_item');
