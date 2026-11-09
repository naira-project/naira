import { defineComponent, h, onMounted, ref, type VNode } from 'vue';

interface HandbookMetadata {
  owner?: string;
  version?: string;
  builtAt?: string;
  source?: { repository?: string; commit?: string };
}

const isHttpUrl = (value: string) => /^https?:\/\//.test(value);
const formatTime = (iso: string) =>
  iso.replace(/^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}).*Z$/, '$1 $2 UTC');

/** Shows owner, source, version and build time from /naira-handbook.json. Renders nothing without it. */
export default defineComponent({
  name: 'HandbookFooter',
  setup() {
    const metadata = ref<HandbookMetadata>();
    onMounted(async () => {
      try {
        const response = await fetch('/naira-handbook.json', { cache: 'no-store' });
        if (response.ok) metadata.value = await response.json();
      } catch {
        // The file only exists in a built site, so a preview has no footer.
      }
    });

    return () => {
      const { owner, version, builtAt, source } = metadata.value ?? {};
      const items: (VNode | string)[] = [];
      if (owner) items.push(`Owner: ${owner}`);
      if (source?.repository) {
        const name = source.repository.replace(/^https?:\/\//, '');
        const label = source.commit ? `${name} @ ${source.commit.slice(0, 7)}` : name;
        items.push(
          isHttpUrl(source.repository)
            ? h('a', { href: source.repository, rel: 'noopener' }, label)
            : label,
        );
      }
      if (version) items.push(`Version ${version}`);
      if (builtAt) items.push(`Built ${formatTime(builtAt)}`);
      if (items.length === 0) return null;

      const children = items.flatMap((item, index) => (index === 0 ? [item] : [' · ', item]));
      return h(
        'footer',
        {
          class: 'naira-handbook-footer',
          style: {
            padding: '16px 24px',
            fontSize: '12px',
            color: 'var(--vp-c-text-2)',
            borderTop: '1px solid var(--vp-c-divider)',
          },
        },
        children,
      );
    };
  },
});
