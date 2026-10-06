const base = import.meta.env.BASE_URL.replace(/\/$/, '');

/** href prefixes an internal path with the site base ("/zever"). */
export function href(path: string): string {
  if (/^(https?:|mailto:|#)/.test(path)) return path;
  return `${base}/${path.replace(/^\//, '')}`;
}

export const GITHUB = 'https://github.com/zenta-dev/zever';
