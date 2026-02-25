import adapter from '@sveltejs/adapter-static';

const dev = process.argv.includes('dev');
const capacitor = process.env.CAPACITOR_BUILD === 'true';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			fallback: 'index.html'
		}),
		paths: {
			base: dev || capacitor ? '' : '/invoices'
		},
		serviceWorker: {
			register: false
		}
	}
};

export default config;
