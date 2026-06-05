import { createCollectionStore } from './collection.svelte';
import type { Client, ClientInput } from '$lib/api/types';

export const clients = createCollectionStore<Client, ClientInput>('clients', 'client');
