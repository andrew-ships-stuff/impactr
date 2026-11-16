import { readdir, readFile, stat } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const dist = resolve('dist');
const htmlFiles = [];
async function walk(dir) {
  for (const name of await readdir(dir)) {
    const path = join(dir, name);
    if ((await stat(path)).isDirectory()) await walk(path);
    else if (name.endsWith('.html')) htmlFiles.push(path);
  }
}
await walk(dist);

const failures = [];
for (const file of htmlFiles) {
  const html = await readFile(file, 'utf8');
  for (const match of html.matchAll(/(?:href|src)="([^"]+)"/g)) {
    const href = match[1];
    if (/^(https?:|mailto:|tel:|data:|#)/.test(href)) continue;
    let pathname = href.split('#')[0].split('?')[0];
    if (!pathname) continue;
    if (pathname.startsWith('/impactr/')) pathname = pathname.slice('/impactr'.length);
    else if (pathname.startsWith('/')) continue;
    else continue; // Astro-generated relative assets are validated by the build.
    let target = join(dist, pathname);
    try {
      const info = await stat(target);
      if (info.isDirectory()) target = join(target, 'index.html');
      await stat(target);
    } catch {
      if (!pathname.match(/\.[a-z0-9]+$/i)) {
        try { await stat(`${target}.html`); continue; } catch {}
      }
      failures.push(`${file.replace(dist, '')}: ${href}`);
    }
  }
}

if (failures.length) {
  console.error(`Broken internal links:\n${failures.join('\n')}`);
  process.exit(1);
}
console.log(`Checked ${htmlFiles.length} HTML files: no broken internal links.`);
