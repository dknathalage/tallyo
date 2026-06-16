import { createCollectionStore } from './collection.svelte';
import type { Participant, ParticipantInput } from '$lib/api/types';

export const participants = createCollectionStore<Participant, ParticipantInput>(
	'participants',
	'participant'
);
