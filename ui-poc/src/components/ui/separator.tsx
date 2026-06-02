import React from 'react';
import * as SeparatorPrimitive from '@radix-ui/react-separator';
import { cn } from '../../lib/utils';

interface SeparatorProps extends React.ComponentPropsWithoutRef<typeof SeparatorPrimitive.Root> {
  className?: string;
}

export function Separator({ className, orientation = 'horizontal', ...props }: SeparatorProps) {
  return (
    <SeparatorPrimitive.Root
      orientation={orientation}
      className={cn(
        'shrink-0 bg-white/10',
        orientation === 'horizontal' ? 'h-px w-full' : 'h-full w-px',
        className
      )}
      {...props}
    />
  );
}
