import nodeAdapter from '@sveltejs/adapter-node';
import staticAdapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { mdsvex } from 'mdsvex';

const isDocs = (process.env.TALLYO_TARGET ?? 'app') === 'docs';
const docsBase = process.env.TALLYO_PAGES_BASE ?? '';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	extensions: ['.svelte', '.md'],
	preprocess: [vitePreprocess(), mdsvex({ extensions: ['.md'] })],
	kit: isDocs
		? {
				adapter: staticAdapter({
					pages: 'build-docs',
					assets: 'build-docs',
					fallback: '404.html',
					strict: false
				}),
				files: {
					routes: 'src/routes-docs',
					hooks: { server: 'src/hooks.docs.server' }
				},
				paths: { base: docsBase },
				prerender: {
					entries: ['*'],
					handleHttpError: ({ status, path, referrer }) => {
						if (status === 404 && /\/console(\/|$)/.test(path)) return;
						throw new Error(`${path} (linked from ${referrer})`);
					}
				}
			}
		: {
				adapter: nodeAdapter({ out: 'build' })
			}
};

export default config;
