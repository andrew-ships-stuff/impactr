export const BASE = import.meta.env.BASE_URL.replace(/\/$/, '');
export const url = (path = '/') => `${BASE}${path.startsWith('/') ? path : `/${path}`}`;

export const docsNav = [
  { group: 'Start here', items: [['Overview', '/docs/'], ['Getting started', '/docs/getting-started/'], ['Installation', '/docs/installation/']] },
  { group: 'Use Impactr', items: [['CLI reference', '/docs/cli/'], ['Configuration', '/docs/configuration/'], ['Graph views', '/docs/graph-views/'], ['Git hooks', '/docs/git-hooks/'], ['JSON output', '/docs/json-output/']] },
  { group: 'Automation', items: [['GitHub Actions', '/docs/github-actions/'], ['GitLab CI', '/docs/gitlab-ci/']] },
  { group: 'Understand', items: [['Architecture', '/docs/architecture/'], ['Embedded inference', '/docs/embedded-inference/'], ['External providers', '/docs/external-providers/'], ['Security', '/docs/security/'], ['Troubleshooting', '/docs/troubleshooting/']] },
] as const;
