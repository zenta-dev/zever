/** Shared SEO constants and JSON-LD builders (landing and docs). */
export const SITE = 'https://zenta-dev.github.io/zever';
export const NAME = 'Zever';
export const TAGLINE = 'One schema. Every backend concern. Zero rewiring.';
export const DESCRIPTION =
  'Zever is a schema-driven scaffolding compiler for Go. Describe services in a .zen schema and it compiles typed ORM, OpenAPI, migrations and routing, then wires swappable adapters (db, cache, queue, auth, storage and more) through one container. It never generates your business logic.';
export const REPO = 'https://github.com/zenta-dev/zever';
export const KEYWORDS = 'Go, Golang, backend framework, schema compiler, code generation, scaffolding, ORM, OpenAPI, migrations, dependency injection, adapters, DSL, MCP';

export const abs = (path: string) => `${SITE}/${path.replace(/^\//, '')}`;

const org = {
  '@type': 'Organization',
  '@id': `${SITE}/#org`,
  name: 'Zenta Dev',
  url: 'https://github.com/zenta-dev',
  logo: { '@type': 'ImageObject', url: abs('icon-512.png'), width: 512, height: 512 },
  sameAs: ['https://github.com/zenta-dev', REPO],
};

const website = {
  '@type': 'WebSite',
  '@id': `${SITE}/#website`,
  url: `${SITE}/`,
  name: NAME,
  description: DESCRIPTION,
  inLanguage: 'en',
  publisher: { '@id': `${SITE}/#org` },
};

/** softwareNode describes Zever itself; reused as `about` on docs pages. */
export const softwareNode = (version?: string) => ({
  '@type': ['SoftwareApplication', 'SoftwareSourceCode'],
  '@id': `${SITE}/#software`,
  name: NAME,
  alternateName: 'zever',
  description: DESCRIPTION,
  url: `${SITE}/`,
  applicationCategory: 'DeveloperApplication',
  applicationSubCategory: 'Code generator and backend framework',
  operatingSystem: 'Linux, macOS, Windows',
  programmingLanguage: { '@type': 'ComputerLanguage', name: 'Go' },
  runtimePlatform: 'Go 1.27+',
  codeRepository: REPO,
  license: 'https://www.apache.org/licenses/LICENSE-2.0',
  isAccessibleForFree: true,
  ...(version ? { softwareVersion: version.replace(/^v/, '') } : {}),
  offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
  author: { '@id': `${SITE}/#org` },
  publisher: { '@id': `${SITE}/#org` },
  image: abs('og.png'),
  featureList: [
    'Schema-driven code generation from .zen files',
    'Typed generics ORM, OpenAPI and migration output',
    'Swappable adapters resolved through one lazy container',
    'Zero-infrastructure defaults (sqlite, memory, log)',
    'Agent-friendly CLI with dry-run, MCP codegen backend and agent skill',
  ],
});

export const landingGraph = (version: string) => ({ '@context': 'https://schema.org', '@graph': [org, website, softwareNode(version)] });

export interface Crumb { name: string; url: string }

export const docsGraph = (p: { title: string; description: string; url: string; image: string; crumbs: Crumb[]; version?: string }) => ({
  '@context': 'https://schema.org',
  '@graph': [
    {
      '@type': 'TechArticle',
      '@id': `${p.url}#article`,
      headline: p.title,
      description: p.description,
      url: p.url,
      mainEntityOfPage: p.url,
      image: p.image,
      inLanguage: 'en',
      isAccessibleForFree: true,
      author: { '@id': `${SITE}/#org` },
      publisher: { '@id': `${SITE}/#org` },
      isPartOf: { '@id': `${SITE}/#website` },
      about: { '@id': `${SITE}/#software` },
    },
    {
      '@type': 'BreadcrumbList',
      itemListElement: p.crumbs.map((c, i) => ({ '@type': 'ListItem', position: i + 1, name: c.name, item: c.url })),
    },
    org,
    website,
    softwareNode(p.version),
  ],
});

export const jsonLd = (data: unknown) => JSON.stringify(data).replace(/</g, '\\u003c');
