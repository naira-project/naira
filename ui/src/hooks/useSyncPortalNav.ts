import * as LuigiClient from '@luigi-project/client';
import { useEffect } from 'react';

const PORTAL_ROOT = '/naira';

/**
 * Highlights the portal sidebar entry for the given catalog viewpoint path
 * (e.g. "model") without reloading the micro frontend.
 * No-op when the UI is not embedded in the portal.
 */
export function useSyncPortalNav(viewpointPath: string | undefined) {
  useEffect(() => {
    if (!viewpointPath || window.parent === window) {
      return;
    }
    LuigiClient.linkManager().withoutSync().navigate(`${PORTAL_ROOT}/${viewpointPath}`);
  }, [viewpointPath]);
}
