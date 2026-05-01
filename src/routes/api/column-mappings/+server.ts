import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import type { CreateColumnMappingInput } from '$lib/repositories/interfaces/ColumnMappingsRepository.js';

export const GET: RequestHandler = async ({ url }) => {
	const entity = url.searchParams.get('entity') ?? 'catalog';
	return json(await repositories.columnMappings.getColumnMappings(entity));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = (await request.json()) as CreateColumnMappingInput;
	const id = await repositories.columnMappings.createColumnMapping(data);
	return json({ id }, { status: 201 });
};

export const DELETE: RequestHandler = async ({ url }) => {
	const id = parseInt(url.searchParams.get('id') ?? '0');
	await repositories.columnMappings.deleteColumnMapping(id);
	return json({ success: true });
};
