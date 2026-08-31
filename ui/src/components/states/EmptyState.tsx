import StatePlaceholder from './StatePlaceholder';

/**
 * Shown as the empty page, after respective plugins for a viewpoint have been
 * run. If the page does not consist of any data for the respective user, then this state comes up.
 */
export default function EmptyState() {
  return (
    <StatePlaceholder
      imageSrc="/empty.svg"
      imageAlt="Ghost image for showing the empty state"
      description="There are no data that could be fetched from the plugins."
    />
  );
}
