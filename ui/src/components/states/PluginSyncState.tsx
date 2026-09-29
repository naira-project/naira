import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { pluginsPageLink } from '../../lib/utils';
import StatePlaceholder from './StatePlaceholder';

/**
 * Shown in place of a data view (e.g. the catalog table) when there is
 * nothing to display yet. Once a sync populates data, the caller swaps
 * this out for the real content.
 */
export default function PluginSyncState({ pluginNames }: { pluginNames?: string[] }) {
  const description: ReactNode =
    pluginNames && pluginNames.length > 0 ? (
      <>
        Please sync the{' '}
        {pluginNames.map((name, i) => (
          <span key={name}>
            {i > 0 && ', '}
            <strong>{name}</strong>
          </span>
        ))}{' '}
        plugin{pluginNames.length > 1 ? 's' : ''} to see the data.{' '}
        <Link to={pluginsPageLink(pluginNames)} className="underline">
          Go to Plugins
        </Link>
      </>
    ) : (
      'Please sync the plugins to see the data.'
    );

  return (
    <StatePlaceholder
      imageSrc="/sync.png"
      imageAlt="Plugin for syncing the data"
      description={description}
    />
  );
}
