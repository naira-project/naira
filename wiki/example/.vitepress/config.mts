import { copyFileSync, mkdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { basename, join } from 'node:path';
import { defineConfig } from 'vitepress';

const require = createRequire(import.meta.url);
const pkg = require('../package.json');

// The portal is light, so the wiki starts light and a visitor who picks a theme keeps it. VitePress
// stores "auto" on the first visit, so the stored value alone cannot tell that nobody chose yet:
// a marker records that the default was applied. It runs before VitePress reads the theme.
const DEFAULT_LIGHT = `try {
  if (!localStorage.getItem('naira-wiki-default-theme')) {
    localStorage.setItem('naira-wiki-default-theme', 'light');
    localStorage.setItem('vitepress-theme-appearance', 'light');
  }
} catch {}`;

export default defineConfig({
  title: pkg.naira.title,
  description: pkg.description,
  lang: 'en-US',
  srcDir: 'docs',
  outDir: 'dist',
  // Page URLs must stay the same across rebuilds and generator upgrades: keep base and cleanUrls.
  base: '/',
  cleanUrls: true,
  head: [['script', {}, DEFAULT_LIGHT]],
  // ignoreDeadLinks stays at its default so a broken internal link fails the build.
  themeConfig: {
    nav: [
      { text: 'Team', link: '/team/about' },
      { text: 'On-call', link: '/oncall/guide' },
      { text: 'Runbooks', link: '/runbooks/restart-the-model-gateway' },
      { text: 'Decisions', link: '/decisions/' },
    ],
    sidebar: [
      {
        text: 'Team',
        items: [
          { text: 'About the team', link: '/team/about' },
          { text: 'Onboarding', link: '/team/onboarding' },
        ],
      },
      { text: 'On-call', items: [{ text: 'On-call guide', link: '/oncall/guide' }] },
      {
        text: 'Runbooks',
        items: [
          { text: 'Restart the model gateway', link: '/runbooks/restart-the-model-gateway' },
          { text: 'Rotate an API key', link: '/runbooks/rotate-an-api-key' },
        ],
      },
      {
        text: 'Decisions',
        items: [
          { text: 'Decision log', link: '/decisions/' },
          { text: '0001 One gateway for all models', link: '/decisions/0001-one-model-gateway' },
        ],
      },
      { text: 'Writing', items: [{ text: 'What you can write', link: '/writing/what-you-can-write' }] },
    ],
    search: { provider: 'local' },
  },
  // The bootstrap page loads the pinned Luigi client from its own origin, never from a CDN.
  buildEnd({ outDir }) {
    const target = join(outDir, '_naira/assets');
    mkdirSync(target, { recursive: true });
    const client = require.resolve('@luigi-project/client');
    for (const file of [client, `${client}.map`]) {
      copyFileSync(file, join(target, basename(file)));
    }
  },
});
