import { copyFileSync, mkdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { basename, join } from 'node:path';
import { defineConfig } from 'vitepress';

const require = createRequire(import.meta.url);
const pkg = require('../package.json');

// The portal is light, so the handbook starts light and a visitor who picks a theme keeps it. VitePress
// stores "auto" on the first visit, so the stored value alone cannot tell that nobody chose yet:
// a marker records that the default was applied. It runs before VitePress reads the theme.
const DEFAULT_LIGHT = `try {
  if (!localStorage.getItem('naira-handbook-default-theme')) {
    localStorage.setItem('naira-handbook-default-theme', 'light');
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
      { text: 'Use Naira', link: '/guide/run-plugins' },
      { text: 'Extend Naira', link: '/build/write-a-plugin' },
    ],
    sidebar: [
      {
        text: 'Get started',
        items: [{ text: 'Find your way around', link: '/guide/find-your-way-around' }],
      },
      {
        text: 'Use Naira',
        items: [
          { text: 'Run plugins', link: '/guide/run-plugins' },
          { text: 'Browse a catalog', link: '/guide/browse-a-catalog' },
          { text: 'Read the tech radar', link: '/guide/read-the-tech-radar' },
          { text: 'Call the catalog API', link: '/guide/call-the-catalog-api' },
        ],
      },
      {
        text: 'Maintain content',
        items: [{ text: 'Update the tech radar', link: '/guide/update-the-tech-radar' }],
      },
      { text: 'Extend Naira', items: [{ text: 'Write a plugin', link: '/build/write-a-plugin' }] },
      { text: 'Reference', items: [{ text: 'Glossary', link: '/reference/glossary' }] },
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
