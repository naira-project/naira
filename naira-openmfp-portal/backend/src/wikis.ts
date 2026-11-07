import { readFileSync } from 'node:fs';
import type { LuigiNode, LuigiNodeCategory } from '@openmfp/portal-server-lib';

/** A wiki as listed in the wikis file. */
export interface Wiki {
  id: string;
  label: string;
  url: string;
  primary: boolean;
  order: number;
}

export interface LoadOptions {
  category: LuigiNodeCategory;
  /** Order of the first wiki node; the others follow in list order. */
  firstOrder: number;
  /** Path segments that wikis must not take. */
  reservedIds: ReadonlySet<string>;
  log: (message: string) => void;
}

// A DNS label: the id is a path segment here and a host name in the wiki's address.
const ID = /^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$/;

function wikiUrl(value: unknown): value is string {
  if (typeof value !== 'string') return false;
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return false;
  }
  const web = url.protocol === 'http:' || url.protocol === 'https:';
  return web && !!url.hostname && !url.username && !url.password && !url.search && !url.hash;
}

function parseWiki(raw: unknown, position: string, seen: Set<string>, reservedIds: ReadonlySet<string>): Wiki {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error(`${position}: must be an object`);
  }
  const { id, label, url, primary = false, order = 0 } = raw as Record<string, unknown>;
  if (typeof id !== 'string' || id === '') throw new Error(`${position}: id is required`);
  if (!ID.test(id)) {
    throw new Error(`${position}: invalid id "${id}": use lowercase letters, digits and "-", at most 42 characters`);
  }
  if (seen.has(id)) throw new Error(`${position}: duplicate id "${id}"`);
  if (reservedIds.has(id)) throw new Error(`${position}: id "${id}" is already a portal page`);
  if (typeof label !== 'string' || label.trim() === '') throw new Error(`${position}: label is required`);
  if (url === undefined) throw new Error(`${position}: url is required`);
  if (!wikiUrl(url)) throw new Error(`${position}: url must be an http(s) URL without credentials, query or fragment`);
  if (typeof primary !== 'boolean') throw new Error(`${position}: primary must be true or false`);
  if (typeof order !== 'number' || !Number.isInteger(order)) throw new Error(`${position}: order must be a whole number`);
  seen.add(id);
  return { id, label, url, primary, order };
}

/**
 * Parses the list of wikis the Naira Helm chart renders (see deploy/charts/naira/README.md)
 * and returns it with the primary wiki first, then by order, then by id.
 * Throws on the first entry that does not fit. Unknown fields are ignored.
 */
export function parseWikis(text: string, reservedIds: ReadonlySet<string>): Wiki[] {
  let list: unknown;
  try {
    list = JSON.parse(text);
  } catch (error) {
    throw new Error(`not valid JSON: ${error instanceof Error ? error.message : error}`);
  }
  if (!Array.isArray(list)) throw new Error('must be a list of wikis');
  const seen = new Set<string>();
  return list
    .map((raw, i) => parseWiki(raw, `[${i}]`, seen, reservedIds))
    .sort((a, b) => Number(b.primary) - Number(a.primary) || a.order - b.order || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/**
 * Maps wikis to Luigi virtual tree nodes, numbered from `firstOrder` in list order.
 * Luigi appends "/:virtualSegment_1" to the node's url, so the url loses its trailing slash.
 */
export function wikiNodes(wikis: Wiki[], category: LuigiNodeCategory, firstOrder: number): LuigiNode[] {
  return wikis.map((wiki, i) => ({
    pathSegment: wiki.id,
    icon: 'assets/icons/wiki.svg',
    label: wiki.label,
    order: firstOrder + i,
    category,
    entityType: 'naira',
    hideFromNav: false,
    url: wiki.url.replace(/\/+$/, ''),
    virtualTree: true,
    loadingIndicator: { enabled: false },
  }));
}

/**
 * Reads the wikis file and returns a node for each wiki. No file name means no wikis.
 * A file that cannot be read or does not fit logs one error and yields no nodes at all.
 */
export function loadWikiNodes(file: string | undefined, options: LoadOptions): LuigiNode[] {
  if (!file) return [];
  try {
    return wikiNodes(parseWikis(readFileSync(file, 'utf8'), options.reservedIds), options.category, options.firstOrder);
  } catch (error) {
    options.log(`No wiki is shown, ${file} is unusable: ${error instanceof Error ? error.message : error}`);
    return [];
  }
}
