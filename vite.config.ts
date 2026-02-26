import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import { defineConfig } from 'vite';

const dev = process.argv.includes('dev');
const basePath = dev ? '' : '/invoices/console';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit(),
		SvelteKitPWA({
			strategies: 'generateSW',
			registerType: 'prompt',
			kit: { adapterFallback: 'index.html' },
			workbox: {
				globPatterns: ['client/**/*.{js,css,ico,png,svg,webp,wasm,webmanifest}'],
				navigateFallback: `${basePath}/index.html`,
				navigateFallbackDenylist: [],
				cleanupOutdatedCaches: true,
				clientsClaim: true,
				skipWaiting: false
			},
			includeAssets: [
				'favicon.svg',
				'robots.txt',
				'sql-wasm.wasm',
				'icons/*.png',
				'icons/*.svg'
			],
			manifest: {
				name: 'Invoice Manager',
				short_name: 'Invoices',
				description: 'A local-first invoice management tool. Your data stays on your device.',
				theme_color: '#2563eb',
				background_color: '#ffffff',
				display: 'standalone',
				start_url: '.',
				scope: '.',
				icons: [
					{ src: 'icons/icon-192x192.png', sizes: '192x192', type: 'image/png' },
					{ src: 'icons/icon-512x512.png', sizes: '512x512', type: 'image/png' },
					{
						src: 'icons/icon-512x512.png',
						sizes: '512x512',
						type: 'image/png',
						purpose: 'maskable'
					},
					{ src: 'icons/icon.svg', sizes: 'any', type: 'image/svg+xml' }
				]
			}
		})
	]
});
