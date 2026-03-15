import { repositories } from '$lib/repositories/sqlite/index.js';
import type { PageServerLoad } from './$types.js';

export const load: PageServerLoad = () => {
  const profile = repositories.businessProfile.getBusinessProfile();
  let apiKeyConfigured = false;
  try {
    const meta = JSON.parse(profile?.metadata ?? '{}');
    apiKeyConfigured = !!meta.anthropic_api_key;
  } catch { /* noop */ }
  return { apiKeyConfigured };
};
