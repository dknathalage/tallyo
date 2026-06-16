import { defineConfig } from 'vitest/config';

export default defineConfig({
	resolve: {
		alias: {
			// Stub SvelteKit virtual modules so pure-TS tests don't need the full kit plugin
			'$app/navigation': new URL('./src/test-stubs/app-navigation.ts', import.meta.url).pathname,
			'$app/state': new URL('./src/test-stubs/app-state.ts', import.meta.url).pathname
		}
	},
	test: {
		environment: 'node'
	}
});
