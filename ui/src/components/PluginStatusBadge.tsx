import { CheckCircle, Clock, Loader2, XCircle } from 'lucide-react';
import { Badge } from './ui/badge';

interface PluginStatusBadgeProps {
  state: 'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'FAILED';
}

/**
 * Visual indicator for plugin run operation state.
 * - PENDING: clock icon, warning color
 * - RUNNING: spinning loader, warning color
 * - SUCCEEDED: checkmark, success color
 * - FAILED: X icon, error color
 */
export function PluginStatusBadge({ state }: PluginStatusBadgeProps) {
  switch (state) {
    case 'PENDING':
      return (
        <Badge variant="warning">
          <Clock size={12} />
          Pending
        </Badge>
      );
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
    case 'FAILED':
      return (
        <Badge variant="error">
          <XCircle size={12} />
          Failed
        </Badge>
      );
  }
}
