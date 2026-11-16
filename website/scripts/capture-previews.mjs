import { chromium } from '@playwright/test';
import { mkdir } from 'node:fs/promises';

const browser = await chromium.launch();
await mkdir('previews', { recursive: true });

const desktop = await browser.newPage({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 });
await desktop.goto('http://127.0.0.1:4325/impactr/', { waitUntil: 'networkidle' });
await desktop.screenshot({ path: 'previews/home-desktop.png', fullPage: true });
await desktop.goto('http://127.0.0.1:4325/impactr/docs/getting-started/', { waitUntil: 'networkidle' });
await desktop.screenshot({ path: 'previews/docs-desktop.png', fullPage: true });

const mobile = await browser.newPage({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 1 });
await mobile.goto('http://127.0.0.1:4325/impactr/', { waitUntil: 'networkidle' });
await mobile.screenshot({ path: 'previews/home-mobile.png', fullPage: true });
await browser.close();
