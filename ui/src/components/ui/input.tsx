import type React from 'react';
import { cn } from '../../lib/utils';

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  startAdornment: React.ReactNode;
}

export function Input({ className, startAdornment, ...props }: InputProps) {
  if (startAdornment) {
    return (
      <div className="relative flex items-center">
        <span className="absolute left-3 text-foreground-secondary">{startAdornment}</span>
        <input
          className={cn(
            'flex h-9 w-full rounded-md border border-gray-300 bg-white pl-9 pr-3 py-1 text-sm shadow-sm placeholder:text-gray-400 focus:outline-none focus:ring-1 focus:ring-primary dark:border-gray-600 dark:bg-background-dark-paper dark:text-foreground-dark-default dark:placeholder:text-gray-500',
            className,
          )}
          {...props}
        />
      </div>
    );
  }
  return (
    <input
      className={cn(
        'flex h-9 w-full rounded-md border border-gray-300 bg-white px-3 py-1 text-sm shadow-sm placeholder:text-gray-400 focus:outline-none focus:ring-1 focus:ring-primary dark:border-gray-600 dark:bg-background-dark-paper dark:text-foreground-dark-default dark:placeholder:text-gray-500',
        className,
      )}
      {...props}
    />
  );
}
