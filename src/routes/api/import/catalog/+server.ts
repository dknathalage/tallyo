import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { getDb } from '$lib/db/connection.js';
import { catalogItems, catalogItemRates, auditLog } from '$lib/db/drizzle-schema.js';
import { eq } from 'drizzle-orm';
import type { DiffResult } from '$lib/import/diff-catalog.js';

export const config = {
	body: { maxSize: '5mb' }
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json() as { diff: DiffResult; options: { updateExisting: boolean } };
	const { diff, options } = body;

	const batchId = crypto.randomUUID();
	let inserted = 0;
	let updated = 0;

	const db = getDb();

	try {
		db.transaction((tx) => {
			for (const row of diff.newItems) {
				const result = tx.insert(catalogItems).values({
					uuid: crypto.randomUUID(),
					name: row.name,
					rate: row.rate,
					unit: row.unit,
					category: row.category,
					sku: row.sku,
					metadata: Object.keys(row.metadata).length > 0 ? JSON.stringify(row.metadata) : '{}'
				}).returning({ id: catalogItems.id }).all();
				const firstResult = result[0];
				if (!firstResult) throw new Error('Failed to insert catalog item');
				const newId = firstResult.id;

				for (const [tierId, tierRate] of Object.entries(row.tierRates)) {
					tx.insert(catalogItemRates).values({
						catalog_item_id: newId,
						rate_tier_id: Number(tierId),
						rate: tierRate as number
					}).onConflictDoUpdate({
						target: [catalogItemRates.catalog_item_id, catalogItemRates.rate_tier_id],
						set: { rate: tierRate as number }
					}).run();
				}

				tx.insert(auditLog).values({
					entity_type: 'catalog',
					entity_id: newId,
					action: 'import',
					changes: JSON.stringify({
						name: { old: null, new: row.name },
						rate: { old: null, new: row.rate },
						sku: { old: null, new: row.sku }
					}),
					batch_id: batchId
				}).run();

				inserted++;
			}

			if (options.updateExisting) {
				for (const item of diff.updatedItems) {
					const row = item.incoming;
					tx.update(catalogItems)
						.set({
							name: row.name,
							rate: row.rate,
							unit: row.unit,
							category: row.category,
							metadata: Object.keys(row.metadata).length > 0 ? JSON.stringify(row.metadata) : '{}'
						})
						.where(eq(catalogItems.id, item.existing.id))
						.run();

					for (const [tierId, tierRate] of Object.entries(row.tierRates)) {
						tx.insert(catalogItemRates).values({
							catalog_item_id: item.existing.id,
							rate_tier_id: Number(tierId),
							rate: tierRate as number
						}).onConflictDoUpdate({
							target: [catalogItemRates.catalog_item_id, catalogItemRates.rate_tier_id],
							set: { rate: tierRate as number }
						}).run();
					}

					tx.insert(auditLog).values({
						entity_type: 'catalog',
						entity_id: item.existing.id,
						action: 'import',
						changes: JSON.stringify({
							name: { old: item.existing.name, new: row.name },
							rate: { old: item.existing.rate, new: row.rate }
						}),
						batch_id: batchId
					}).run();

					updated++;
				}
			}
		});
	} catch (e) {
		console.error('Catalog import failed:', e);
		return error(500, { message: 'Catalog import failed' });
	}

	return json({ inserted, updated, batchId });
};
