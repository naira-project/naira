import type React from 'react';
import { cn } from '../../lib/utils';

interface InputProps extends React.ComponentProps<"input"> {
  startAdornment?: React.ReactNode
  endAdornment?: React.ReactNode
}

function Input({ className, type, startAdornment, endAdornment, ...props }: InputProps) {
  const input = (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 py-1 text-base transition-colors outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:disabled:bg-input/80 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
        startAdornment && "pl-8",
        endAdornment && "pr-8",
        className,
      )}
      {...props}
    />
  )

  if (!startAdornment && !endAdornment) {
    return input
  }

  return (
    <div className="relative flex items-center">
      {startAdornment && (
        <span className="absolute left-2.5 flex items-center text-muted-foreground">
          {startAdornment}
        </span>
      )}
      {input}
      {endAdornment && (
        <span className="absolute right-2.5 flex items-center text-muted-foreground">
          {endAdornment}
        </span>
      )}
    </div>
  )
}

export { Input }
