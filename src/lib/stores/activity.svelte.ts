export interface ActivityProgress {
	bytes: number;
	total: number;
}

export interface ActivityState {
	id: string | null;
	label: string;
	stage: string | null;
	progress: ActivityProgress | null;
}

const initial: ActivityState = { id: null, label: '', stage: null, progress: null };
let state = $state<ActivityState>({ ...initial });

export function getActivity(): ActivityState {
	return state;
}

export function startActivity(label: string): string {
	const id = crypto.randomUUID();
	state = { id, label, stage: null, progress: null };
	return id;
}

export function updateActivity(id: string, patch: Partial<Omit<ActivityState, 'id'>>): void {
	if (state.id !== id) return;
	state = { ...state, ...patch };
}

export function endActivity(id: string): void {
	if (state.id !== id) return;
	state = { ...initial };
}
