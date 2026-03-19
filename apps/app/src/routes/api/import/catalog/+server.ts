import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { getDb } from '$lib/db/connection.js';
import { catalogItems, catalogItemRates } from '$lib/db/drizzle-schema.js';
import { logAudit } from '$lib/db/audit.js';
import { eq, and } from 'drizzle-orm';
import type { DiffResult } from '$lib/import/diff-catalog.js';

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json() as { diff: DiffResult; options: { updateExisting: boolean } };
	const { diff, options } = body;

	const batchId = crypto.randomUUID();
	let inserted = 0;
	let updated = 0;

	const db = getDb();

	await db.transaction(async (tx) => {
		for (const row of diff.newItems) {
			const result = await tx.insert(catalogItems).values({
				uuid: crypto.randomUUID(),
				name: row.name,
				rate: row.rate,
				unit: row.unit,
				category: row.category,
				sku: row.sku,
				metadata: Object.keys(row.metadata).length > 0 ? JSON.stringify(row.metadata) : '{}'
			}).returning({ id: catalogItems.id });
			const newId = result[0].id;

			for (const [tierId, tierRate] of Object.entries(row.tierRates)) {
				await tx.insert(catalogItemRates).values({
					catalog_item_id: newId,
					rate_tier_id: Number(tierId),
					rate: tierRate as number
				}).onConflictDoUpdate({
					target: [catalogItemRates.catalog_item_id, catalogItemRates.rate_tier_id],
					set: { rate: tierRate as number }
				});
			}

			await logAudit({
				entity_type: 'catalog',
				entity_id: newId,
				action: 'import',
				changes: {
					name: { old: null, new: row.name },
					rate: { old: null, new: row.rate },
					sku: { old: null, new: row.sku }
				},
				batch_id: batchId
			});

			inserted++;
		}

		if (options.updateExisting) {
			for (const item of diff.updatedItems) {
				const row = item.incoming;
				await tx.update(catalogItems)
					.set({
						name: row.name,
						rate: row.rate,
						unit: row.unit,
						category: row.category,
						metadata: Object.keys(row.metadata).length > 0 ? JSON.stringify(row.metadata) : '{}'
					})
					.where(eq(catalogItems.id, item.existing.id));

				for (const [tierId, tierRate] of Object.entries(row.tierRates)) {
					await tx.insert(catalogItemRates).values({
						catalog_item_id: item.existing.id,
						rate_tier_id: Number(tierId),
						rate: tierRate as number
					}).onConflictDoUpdate({
						target: [catalogItemRates.catalog_item_id, catalogItemRates.rate_tier_id],
						set: { rate: tierRate as number }
					});
				}

				await logAudit({
					entity_type: 'catalog',
					entity_id: item.existing.id,
					action: 'import',
					changes: {
						name: { old: item.existing.name, new: row.name },
						rate: { old: item.existing.rate, new: row.rate }
					},
					batch_id: batchId
				});

				updated++;
			}
		}
	});

	return json({ inserted, updated, batchId });
};
