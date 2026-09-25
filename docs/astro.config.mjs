import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightThemeBlack from 'starlight-theme-black';

// https://starlight.astro.build/reference/configuration/
export default defineConfig({
	site: 'https://zenta-dev.github.io',
	base: '/zever/',
	integrations: [
		starlight({
			title: 'zever',
			favicon: '/favicon.svg',
			logo: {
				src: './src/assets/logo.svg',
				alt: 'zever logo',
			},
			customCss: ['./src/styles/custom.css'],
			components: {
				Head: './src/components/Head.astro',
			},
			plugins: [
				starlightThemeBlack({
					navLinks: [{ label: 'Docs', link: '/getting-started/installation' }],
				}),
			],
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/zenta-dev/zever' },
			],
			sidebar: [
				{
					label: 'Getting Started',
					items: [
						{ label: 'Introduction', link: '/' },
						{ label: 'Installation', link: 'getting-started/installation' },
						{ label: 'Configuration', link: 'getting-started/configuration' },
						{ label: 'Directory Structure', link: 'getting-started/directory-structure' },
						{ label: 'Deployment', link: 'getting-started/deployment' },
						{ label: 'Upgrade', link: 'getting-started/upgrade' },
					],
				},
				{
					label: 'Tutorials',
					items: [
						{ label: 'Overview', link: 'tutorials/overview' },
						{ label: 'Build a Booking API', link: 'tutorials/build-a-booking-api' },
						{ label: 'Explore the Showcase App', link: 'tutorials/explore-the-showcase-app' },
					],
				},
				{
					label: 'Architecture',
					items: [
						{ label: 'Lifecycle', link: 'architecture/lifecycle' },
						{ label: 'Container', link: 'architecture/container' },
						{ label: 'Providers & Adapters', link: 'architecture/providers-adapters' },
						{ label: 'Compiler Boundary', link: 'architecture/compiler-boundary' },
					],
				},
				{
					label: 'Basics',
					items: [
						{ label: 'Routing', link: 'basics/routing' },
						{ label: 'Middleware', link: 'basics/middleware' },
						{ label: 'Requests & Responses', link: 'basics/requests-responses' },
						{ label: 'Validation', link: 'basics/validation' },
						{ label: 'Errors', link: 'basics/errors-logging' },
						{ label: 'Logging', link: 'basics/logging' },
						{ label: 'Codec', link: 'basics/codec' },
						{ label: 'Sessions', link: 'basics/sessions' },
						{ label: 'Internationalization', link: 'basics/i18n' },
					],
				},
				{
					label: 'Schema DSL (.zen)',
					items: [
						{ label: 'Syntax (.zen)', link: 'dsl/syntax-zen' },
						{ label: 'Resolver & IR', link: 'dsl/resolver-ir' },
						{ label: 'Backends', link: 'dsl/backends' },
						{ label: 'LSP & Tooling', link: 'dsl/lsp-tooling' },
					],
				},
				{
					label: 'Database & ORM',
					items: [
						{ label: 'Query Builder', link: 'database/query-builder' },
						{ label: 'Migrations & Seeding', link: 'database/migrations-seeding' },
						{ label: 'Redis', link: 'database/redis' },
					],
				},
				{
					label: 'Digging Deeper',
					items: [
						{ label: 'Cache', link: 'digging-deeper/cache' },
						{ label: 'Queue', link: 'digging-deeper/queue' },
						{ label: 'Scheduler & Jobs', link: 'digging-deeper/scheduler-jobs' },
						{ label: 'Events & Event Bus', link: 'digging-deeper/events-eventbus' },
						{ label: 'Mailer', link: 'digging-deeper/mailer' },
						{ label: 'Notifications', link: 'digging-deeper/notifications' },
						{ label: 'Storage & Media', link: 'digging-deeper/storage-media' },
						{ label: 'Search', link: 'digging-deeper/search' },
						{ label: 'Feature Flags', link: 'digging-deeper/flags' },
						{ label: 'Geo', link: 'digging-deeper/geo' },
						{ label: 'Rate Limit', link: 'digging-deeper/ratelimit' },
						{ label: 'Lock & Idempotency', link: 'digging-deeper/lock-idempotency' },
						{ label: 'Webhooks', link: 'digging-deeper/webhooks' },
						{ label: 'Workflows', link: 'digging-deeper/workflows' },
						{ label: 'Tenants', link: 'digging-deeper/tenants' },
						{ label: 'Observability', link: 'digging-deeper/observability' },
						{ label: 'AI', link: 'digging-deeper/ai' },
						{ label: 'Analytics', link: 'digging-deeper/analytics' },
						{ label: 'Billing', link: 'digging-deeper/billing' },
						{ label: 'Payments', link: 'digging-deeper/payment' },
						{ label: 'Documents', link: 'digging-deeper/document' },
						{ label: 'Media Processing', link: 'digging-deeper/media' },
						{ label: 'Vector Store', link: 'digging-deeper/vectorstore' },
						{ label: 'Secrets', link: 'digging-deeper/secrets' },
					],
				},
				{
					label: 'Security',
					items: [
						{ label: 'Auth', link: 'security/auth' },
						{ label: 'AuthZ & Permissions', link: 'security/authz-permissions' },
						{ label: 'Password Hashing', link: 'security/password-hashing' },
						{ label: 'Crypto', link: 'security/crypto-secrets' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'CLI', link: 'reference/cli' },
						{ label: 'Config Reference', link: 'reference/config-reference' },
						{ label: 'Adapters Matrix', link: 'reference/adapters-matrix' },
						{ label: 'API Index', link: 'reference/api-index' },
					],
				},
				{ label: 'Contribute', link: 'contribute' },
			],
		}),
	],
});
