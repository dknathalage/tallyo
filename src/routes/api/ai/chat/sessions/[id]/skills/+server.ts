import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { listSkills } from '$lib/server/ai/skills.js';

function parseId(raw: string | undefined): number {
	const n = Number(raw);
	if (!Number.isFinite(n) || n <= 0) throw error(400, 'invalid id');
	return n;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const loaded = await repositories.aiChat.getLoadedSkills(id);
	const available = listSkills().map((s) => ({
		id: s.id,
		title: s.title,
		description: s.description,
		tool_count: s.tools.length,
		alwaysLoaded: s.alwaysLoaded === true,
		loaded: s.alwaysLoaded === true || loaded.includes(s.id)
	}));
	return json({ loaded, available });
};

export const POST: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const body = (await request.json().catch(() => ({}))) as {
		add?: unknown;
		remove?: unknown;
	};
	const add = Array.isArray(body.add) ? body.add.filter((s) => typeof s === 'string') : [];
	const remove = Array.isArray(body.remove) ? body.remove.filter((s) => typeof s === 'string') : [];
	const known = new Set(listSkills().map((s) => s.id));
	const validAdd = (add as string[]).filter((s) => known.has(s));
	const validRemove = new Set((remove as string[]).filter((s) => known.has(s)));

	const current = await repositories.aiChat.getLoadedSkills(id);
	const merged = new Set(current);
	for (const a of validAdd) merged.add(a);
	for (const r of validRemove) merged.delete(r);

	await repositories.aiChat.setLoadedSkills(id, Array.from(merged));
	return json({ loaded: Array.from(merged) });
};
