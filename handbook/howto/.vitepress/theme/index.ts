import { inBrowser, type Theme } from 'vitepress';
import DefaultTheme from 'vitepress/theme';
import { h } from 'vue';
import { setupLuigiBridge } from './luigi-bridge';
import HandbookFooter from './handbook-footer';

export default {
  extends: DefaultTheme,
  Layout: () => h(DefaultTheme.Layout, null, { 'layout-bottom': () => h(HandbookFooter) }),
  enhanceApp({ router }) {
    if (inBrowser) setupLuigiBridge(router);
  },
} satisfies Theme;
