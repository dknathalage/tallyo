import { apiGet, tenantPath } from '$lib/api/client';

/**
 * Singleton feature-gate store. Mirrors the backend's GET /api/features map
 * (camelCase keys → bool). The layout calls load() once on bootstrap; pages
 * read `features.agent` etc. to hide affordances the server has gated off.
 * Unknown/unloaded keys read false, so UI stays hidden until proven enabled.
 */
function createFeaturesStore() {
	let flags = $state<Record<string, boolean>>({});

	async function load(): Promise<void> {
		const got = await apiGet<Record<string, boolean>>(tenantPath('features'));
		if (got !== null) {
			flags = got;
		}
	}

	return {
		load,
		get agent(): boolean {
			return flags.agent === true;
		}
	};
}

export const features = createFeaturesStore();
