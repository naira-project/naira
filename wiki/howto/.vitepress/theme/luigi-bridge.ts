import type { Router } from 'vitepress';

const SESSION_PATH = '/_naira/session';
const TOKEN_ISSUED = 'luigi.auth.tokenIssued';

async function sendToken(token: string): Promise<void> {
  const response = await fetch(SESSION_PATH, {
    method: 'POST',
    headers: { 'x-naira-token': token },
  });
  if (!response.ok) throw new Error(`${SESSION_PATH} answered ${response.status}`);
}

type LuigiClientApi = typeof import('@luigi-project/client');

// The package is a UMD bundle. A built site exposes its API only on `default`.
async function loadLuigiClient(): Promise<LuigiClientApi> {
  const module: LuigiClientApi & { default?: LuigiClientApi } = await import(
    '@luigi-project/client'
  );
  return module.default ?? module;
}

// The portal sends the token in the context. Luigi's own auth data has no `accessToken` there,
// so getToken() stays empty and is only a fallback.
function portalToken(client: LuigiClientApi): string | undefined {
  for (const token of [client.getContext()?.token, client.getToken()] as unknown[]) {
    if (typeof token === 'string' && token !== '') return token;
  }
  return undefined;
}

/**
 * Connects the site to the Naira portal when it runs in a Luigi iframe: in-site navigation is
 * mirrored into the portal URL, and the session cookie follows the access token. Does nothing
 * outside an iframe. Browser only, because the Luigi client needs `window`. Never rejects.
 */
export async function setupLuigiBridge(router: Router): Promise<void> {
  if (window.parent === window) return;
  try {
    const LuigiClient = await loadLuigiClient();
    let sentToken: string | undefined;
    let syncedPath: string | undefined;

    const syncToken = async (token: string | undefined) => {
      if (!token || token === sentToken) return;
      sentToken = token;
      try {
        await sendToken(token);
      } catch (error) {
        sentToken = undefined;
        console.warn('[naira] could not refresh the wiki session', error);
      }
    };

    LuigiClient.addInitListener(() => syncToken(portalToken(LuigiClient)));
    LuigiClient.addContextUpdateListener(() => syncToken(portalToken(LuigiClient)));
    // The client stores a refreshed token without telling its listeners, so watch the message.
    window.addEventListener('message', (event) => {
      if (event.source === window.parent && event.data?.msg === TOKEN_ISSUED) {
        syncToken(event.data.authData?.accessToken);
      }
    });

    // The router has a single hook slot: keep whatever is already there.
    const previous = router.onAfterRouteChange;
    router.onAfterRouteChange = async (to) => {
      await previous?.(to);
      if (!LuigiClient.isLuigiClientInitialized()) return;
      const { pathname, search, hash } = new URL(to, window.location.origin);
      const path = pathname + search + hash;
      if (path === syncedPath) return;
      syncedPath = path;
      LuigiClient.linkManager().fromVirtualTreeRoot().withoutSync().navigate(path);
    };
  } catch (error) {
    console.warn('[naira] the Luigi bridge did not start', error);
  }
}
