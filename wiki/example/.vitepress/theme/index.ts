import { inBrowser, type Theme } from 'vitepress';
import DefaultTheme from 'vitepress/theme';
import { h } from 'vue';
import { setupLuigiBridge } from './luigi-bridge';
import WikiFooter from './wiki-footer';

export default {
  extends: DefaultTheme,
  Layout: () => h(DefaultTheme.Layout, null, { 'layout-bottom': () => h(WikiFooter) }),
  enhanceApp({ router }) {
    if (inBrowser) setupLuigiBridge(router);
  },
} satisfies Theme;
