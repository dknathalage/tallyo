import { createCollectionStore } from './collection.svelte';
import type { RateTier, RateTierInput } from '$lib/api/types';

export const rateTiers = createCollectionStore<RateTier, RateTierInput>('rate-tiers', 'rate_tier');
