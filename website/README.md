# Impactr website

The official Impactr website is an Astro static site built for GitHub Pages. Product documentation is grounded in the repository's implemented CLI, configuration, integrations, and security guidance.

## Develop locally

Requirements: Node.js 20 or newer and npm.

```bash
cd website
npm install
npm run dev
```

The default base path is `/impactr/`, matching `https://andrew-ships-stuff.github.io/impactr/`. Override it for a custom-domain build with `ASTRO_BASE=/ npm run build`.

## Validate

```bash
npm run check
npm run build
npm run links
npx playwright install chromium
npx playwright test
```

The build is fully static. Pagefind creates the documentation search index after Astro generation. Playwright covers graph controls, theme persistence, docs navigation, search focus, and the mobile menu.

## Structure

- `src/pages/` contains marketing, architecture, integration, security, example, 404, and MDX documentation routes.
- `src/components/GraphExplorer.astro` is the client-side fixture used by the signature demo.
- `public/brand/` contains original SVG logo variants.
- `scripts/check-links.mjs` validates base-aware internal links in the production output.
- `tests/` contains critical browser flows.

## Deployment

`.github/workflows/pages.yml` uses the official Astro and GitHub Pages actions. It runs when website files change on `master` and can also be dispatched manually.

Before the first deployment, set the repository's **Settings → Pages → Source** to **GitHub Actions**. The expected URL is:

```text
https://andrew-ships-stuff.github.io/impactr/
```

The workflow is configured, but a successful live deployment must be verified in GitHub after merge or manual dispatch.

## Product-source notes

- The current repository and Homebrew tap owner is `andrew-ships-stuff`.
- Model weights are installed separately with `impactr model install`; analysis does not download them implicitly.
- Fork PR comments require an artifact-only run plus a separately trusted publisher. The one-job example is only suitable for trusted same-repository pull requests.
- Browser demos use clearly labeled deterministic fixture data and do not inspect visitors' repositories.
