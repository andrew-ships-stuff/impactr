import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('homepage explains the product and graph tabs work', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: /See the impact before you merge/i })).toBeVisible();
  await expect(page.getByText('brew install andrew-ships-stuff/tap/impactr').first()).toBeVisible();
  const impactTab = page.getByRole('tab', { name: 'Impact radius' });
  await impactTab.click();
  await expect(impactTab).toHaveAttribute('aria-selected', 'true');
  const graphNode = page.getByRole('button', { name: /Redis, Added/ });
  const mobileNode = page.locator('[data-mobile-node="redis"]');
  if (await graphNode.isVisible()) await graphNode.click(); else await mobileNode.click();
  await expect(page.locator('[data-inspect-label]')).toHaveText('Redis');
});

test('theme choice persists across navigation', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Switch color theme' }).click();
  const theme = await page.locator('html').getAttribute('data-theme');
  await page.goto('/docs/');
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme ?? 'light');
});

test('documentation navigation and search trigger are keyboard accessible', async ({ page }, testInfo) => {
  await page.goto('/docs/');
  await expect(page.getByRole('heading', { name: 'Impactr documentation' })).toBeVisible();
  await page.keyboard.press('/');
  await expect(page.getByLabel('Search Impactr docs')).toBeFocused();
  await page.keyboard.press('Escape');
  if (testInfo.project.name === 'mobile') await page.getByRole('button', { name: 'Documentation menu' }).click();
  await page.getByRole('navigation', { name: 'Documentation navigation' }).getByRole('link', { name: 'CLI reference' }).click();
  await expect(page.getByRole('heading', { name: 'CLI reference' })).toBeVisible();
});

test('mobile navigation opens and exposes routes', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'mobile', 'mobile-only flow');
  await page.goto('/');
  const menu = page.getByRole('button', { name: 'Open navigation' });
  await menu.click();
  await expect(page.locator('#mobile-nav')).toBeVisible();
  await expect(page.locator('#mobile-nav').getByRole('link', { name: 'Examples' })).toBeVisible();
});

test('homepage has no serious accessibility violations', async ({ page }) => {
  await page.goto('/');
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations.filter((violation) => ['serious', 'critical'].includes(violation.impact ?? ''))).toEqual([]);
});
