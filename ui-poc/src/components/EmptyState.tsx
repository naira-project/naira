interface EmptyStateProps {
  title?: string;
  description?: string;
}

/**
 * Shown in place of a data view (e.g. the catalog table) when there is
 * nothing to display yet. Once a sync populates data, the caller swaps
 * this out for the real content.
 */
export default function EmptyState({
  title = 'This Page is Empty',
  description = 'Please sync the plugins to see the data',
}: EmptyStateProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-[10px] self-stretch pt-10 pb-[220px]">
      <div className="flex h-[262.381px] w-[280.603px] items-center justify-center pt-[31.925px] pr-[35.496px] pb-[30.226px] pl-[33.65px]">
        <img
          src="/sync.png"
          alt="Plugin image for syncing the data"
          className="h-full w-full object-contain"
        />
      </div>

      <div className="flex w-[331px] flex-col items-center gap-[24px]">
        <h3 className="text-center font-sans text-2xl font-semibold leading-6 text-[var(--Side-Bar-Text,#C2C0B6)]">
          {title}
        </h3>
        <p className="text-center font-sans text-xl font-medium leading-6 text-[var(--Side-Bar-Text,#C2C0B6)]">
          {description}
        </p>
      </div>
    </div>
  );
}
