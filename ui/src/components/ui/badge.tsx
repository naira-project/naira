import { cva } from 'class-variance-authority';
import type React from 'react';
import { cn } from '../../lib/utils';

const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-white',
        secondary: 'border-transparent bg-secondary text-white',
        outline: 'border-primary text-primary bg-transparent',
        success: 'border-transparent bg-green-500 text-white',
        warning: 'border-transparent bg-orange-400 text-white',
        error: 'border-transparent bg-red-500 text-white',
        destructive: 'border-transparent bg-red-500 text-white',
      },
    },
    defaultVariants: { variant: 'default' },
  },
);

interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'error' | 'destructive';
}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
