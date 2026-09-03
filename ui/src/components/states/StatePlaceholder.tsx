import type { ReactNode } from 'react';

interface StatePlaceholderProps {
  imageSrc: string;
  imageAlt: string;
  title?: string;
  description: ReactNode;
}

/**
 * Shared layout for full-panel placeholder states
 */
export default function StatePlaceholder({
  imageSrc,
  imageAlt,
  title = 'This Page is Empty',
  description,
}: StatePlaceholderProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-[10px] self-stretch pt-10 pb-[220px]">
      <div className="flex h-[262.381px] w-[280.603px] items-center justify-center pt-[31.925px] pr-[35.496px] pb-[30.226px] pl-[33.65px]">
        <img src={imageSrc} alt={imageAlt} className="h-full w-full object-contain" />
      </div>

      <div className="flex w-[331px] flex-col items-center gap-[24px]">
        <h3 className="text-center font-sans text-2xl font-semibold leading-6 text-[var(--placeholder-text)]">
          {title}
        </h3>
        <p className="text-center font-sans text-xl font-medium leading-6 text-[var(--placeholder-text)]">
          {description}
        </p>
      </div>
    </div>
  );
}
