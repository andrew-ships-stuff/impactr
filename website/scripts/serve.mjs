import { createServer } from 'node:http';
import { readFile, stat } from 'node:fs/promises';
import { extname, join, normalize, resolve } from 'node:path';

const root = resolve('dist');
const port = Number(process.env.PORT || 4325);
const types = { '.html': 'text/html; charset=utf-8', '.css': 'text/css', '.js': 'text/javascript', '.svg': 'image/svg+xml', '.png': 'image/png', '.json': 'application/json', '.wasm': 'application/wasm' };

createServer(async (request, response) => {
  const rawPath = new URL(request.url || '/', `http://${request.headers.host}`).pathname;
  const stripped = rawPath.replace(/^\/impactr(?=\/|$)/, '') || '/';
  let file = normalize(join(root, stripped));
  if (!file.startsWith(root)) { response.writeHead(403).end('Forbidden'); return; }
  try {
    if ((await stat(file)).isDirectory()) file = join(file, 'index.html');
    const body = await readFile(file);
    response.writeHead(200, { 'Content-Type': types[extname(file)] || 'application/octet-stream' }).end(body);
  } catch {
    try { response.writeHead(404, { 'Content-Type': 'text/html' }).end(await readFile(join(root, '404.html'))); }
    catch { response.writeHead(404).end('Not found'); }
  }
}).listen(port, '127.0.0.1', () => console.log(`Static preview: http://127.0.0.1:${port}/impactr/`));
