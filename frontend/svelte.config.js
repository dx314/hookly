import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: false  // Allow non-prerendered routes for SPA
		}),
		alias: {
			'$api': 'src/api',
			'$components': 'src/lib/components'
		}
	}
};

export default config;
