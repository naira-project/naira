import { copyFileSync, mkdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { basename, join } from 'node:path';
import { defineConfig } from 'vitepress';

const require = createRequire(import.meta.url);
const pkg = require('../package.json');

export default defineConfig({
  title: pkg.naira.title,
  description: pkg.description,
  lang: 'en-US',
  srcDir: 'docs',
  outDir: 'dist',
  // Page URLs must stay the same across rebuilds and generator upgrades: keep base and cleanUrls.
  base: '/',
  cleanUrls: true,
  // ignoreDeadLinks stays at its default so a broken internal link fails the build.
  themeConfig: {
    nav: [{ text: 'Guide', link: '/guide/getting-started' }],
    sidebar: [
      { text: 'Guide', items: [{ text: 'Getting started', link: '/guide/getting-started' }] },
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
