import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import { readFileSync, existsSync } from 'fs';
import { execSync } from 'child_process';

const pkg = JSON.parse(readFileSync('./package.json', 'utf-8'));

function getGitSha(): string {
	try {
		return execSync('git rev-parse --short HEAD', { encoding: 'utf-8' }).trim();
	} catch {
		// Fall back to a build-sha file written by the update script
		if (existsSync('.build-sha')) return readFileSync('.build-sha', 'utf-8').trim();
		return 'unknown';
	}
}

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	define: {
		__PKG_VERSION__: JSON.stringify(getGitSha())
	},
	test: {
		include: ['src/**/*.test.ts'],
		coverage: {
			provider: 'v8',
			reporter: ['text', 'json', 'html'],
			include: ['src/lib/**/*.ts'],
			exclude: ['src/lib/**/*.test.ts', 'src/lib/types/**'],
			thresholds: {
				lines: 80,
				functions: 80,
				branches: 65,
				statements: 80
			}
		}
	}
});
