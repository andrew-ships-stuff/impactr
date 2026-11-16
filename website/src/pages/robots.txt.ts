import type { APIRoute } from 'astro';
export const GET: APIRoute = () => new Response('User-agent: *\nAllow: /\nSitemap: https://andrew-ships-stuff.github.io/impactr/sitemap-index.xml\n', { headers: { 'Content-Type': 'text/plain' } });
