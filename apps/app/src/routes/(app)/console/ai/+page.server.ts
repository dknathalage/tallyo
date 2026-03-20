import { repositories } from '$lib/repositories/index.js';
import type { PageServerLoad } from './$types.js';

export const load: PageServerLoad = async () => {
  const profile = await repositories.businessProfile.getBusinessProfile();
  let apiKeyConfigured = false;
  try {
    const meta = JSON.parse(profile?.metadata ?? '{}');
    apiKeyConfigured = !!meta.anthropic_api_key;
  } catch { /* noop */ }
  return { apiKeyConfigured };
};
