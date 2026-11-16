import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import sitemap from '@astrojs/sitemap';
import tailwindcss from '@tailwindcss/vite';

const base = process.env.ASTRO_BASE ?? '/impactr';

export default defineConfig({
  site: 'https://andrew-ships-stuff.github.io',
  base,
  output: 'static',
  integrations: [mdx(), sitemap()],
  markdown: {
    shikiConfig: {
      themes: { light: 'github-light', dark: 'github-dark-default' },
      wrap: true,
    },
  },
  vite: { plugins: [tailwindcss()] },
});
