const SESSION_PATH = '/_naira/session';
const INIT_TIMEOUT_MS = 10000;

const hasControlCharacter = (value) =>
  [...value].some((char) => char.charCodeAt(0) < 0x20 || char.charCodeAt(0) === 0x7f);

/**
 * Returns the absolute URL to open once the session cookie is set. Only paths on `origin`
 * are accepted. Anything else, and the /_naira/ pages themselves, resolve to the site root.
 */
export function resolveReturnUrl(value, origin) {
  const root = `${origin}/`;
  if (typeof value !== 'string' || !value.startsWith('/')) return root;
  // The URL parser drops tabs and newlines, so "/<TAB>/host" would otherwise become "//host".
  if (hasControlCharacter(value) || /^\/[/\\]/.test(value)) return root;
  let url;
  try {
    url = new URL(value, origin);
  } catch {
    return root;
  }
  if (url.origin !== origin) return root;
  if (url.pathname === '/_naira' || url.pathname.startsWith('/_naira/')) return root;
  return url.href;
}

const ATTEMPTS_KEY = 'naira-wiki-sign-in-attempts';
const MAX_ATTEMPTS = 3;
const ATTEMPT_WINDOW_MS = 30000;

/**
 * Takes the stored sign-in attempt times and returns the times to store next, plus whether
 * `MAX_ATTEMPTS` attempts already happened in the last `ATTEMPT_WINDOW_MS`. An attempt that
 * is refused is not recorded, so the block ends once the earlier attempts age out.
 */
export function recordAttempt(stored, now) {
  const recent = (Array.isArray(stored) ? stored : []).filter(
    (time) => Number.isFinite(time) && now - time < ATTEMPT_WINDOW_MS,
  );
  const exhausted = recent.length >= MAX_ATTEMPTS;
  return { attempts: exhausted ? recent : [...recent, now], exhausted };
}

function tooManyAttempts() {
  try {
    const { attempts, exhausted } = recordAttempt(
      JSON.parse(sessionStorage.getItem(ATTEMPTS_KEY)),
      Date.now(),
    );
    sessionStorage.setItem(ATTEMPTS_KEY, JSON.stringify(attempts));
    return exhausted;
  } catch {
    return false; // Without storage there is no guard.
  }
}

async function openSession(token) {
  const response = await fetch(SESSION_PATH, {
    method: 'POST',
    headers: { 'x-naira-token': token },
  });
  if (!response.ok) throw new Error(`${SESSION_PATH} answered ${response.status}`);
}

function run() {
  const status = document.getElementById('status');
  const show = (text) => {
    status.textContent = text;
  };
  const luigi = globalThis.LuigiClient;
  if (!luigi) return show('The Naira client script did not load. Reload the page.');
  if (window.parent === window)
    return show('This wiki opens inside Naira. Open it from the sidebar.');

  const target = resolveReturnUrl(
    new URLSearchParams(location.search).get('return'),
    location.origin,
  );
  // A browser that drops the session cookie would otherwise bounce between this page and the wiki.
  if (tooManyAttempts()) {
    return show(
      'The browser did not keep the wiki session. Check that cookies are allowed and that the wiki address is on the same site as Naira.',
    );
  }
  const timeout = setTimeout(() => show('Naira did not answer. Reload the page.'), INIT_TIMEOUT_MS);
  luigi.addInitListener(async () => {
    clearTimeout(timeout);
    const token = luigi.getToken();
    if (!token) return show('Naira did not provide a session. Sign in to Naira again.');
    try {
      await openSession(token);
      location.replace(target);
    } catch (error) {
      console.error(error);
      show('Could not sign in to this wiki. Reload the page.');
    }
  });
}

if (typeof document !== 'undefined') run();
