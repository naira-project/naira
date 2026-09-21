import { CheckCircle, Clock, Loader2, XCircle } from 'lucide-react';
import { Badge } from './ui/badge';

interface PluginStatusBadgeProps {
  state: 'RUNNING' | 'SUCCEEDED';
}

/**
 * Visual indicator for plugin run operation state.
 * - RUNNING: spinning loader, warning color
 * - SUCCEEDED: checkmark, success color
 */
export function PluginStatusBadge({ state }: PluginStatusBadgeProps) {
  switch (state) {
    case 'RUNNING':
      return (
        <Badge variant="warning">
          <Loader2 size={12} className="animate-spin" />
          Running
        </Badge>
      );
    case 'SUCCEEDED':
      return (
        <Badge variant="success">
          <CheckCircle size={12} />
          Success
        </Badge>
      );
  }
}
