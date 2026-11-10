import { readFileSync } from 'node:fs';
import type { LuigiNode, LuigiNodeCategory } from '@openmfp/portal-server-lib';

/** A handbook as listed in the handbooks file. */
export interface Handbook {
  id: string;
  label: string;
  url: string;
  primary: boolean;
  order: number;
}

export interface LoadOptions {
  category: LuigiNodeCategory;
  /** Order of the first handbook node; the others follow in list order. */
  firstOrder: number;
  /** Path segments that handbooks must not take. */
  reservedIds: ReadonlySet<string>;
  log: (message: string) => void;
}

// A DNS label: the id is a path segment here and a host name in the handbook's address.
const ID = /^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$/;

function handbookUrl(value: unknown): value is string {
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

function parseHandbook(raw: unknown, position: string, seen: Set<string>, reservedIds: ReadonlySet<string>): Handbook {
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
  if (!handbookUrl(url)) throw new Error(`${position}: url must be an http(s) URL without credentials, query or fragment`);
  if (typeof primary !== 'boolean') throw new Error(`${position}: primary must be true or false`);
  if (typeof order !== 'number' || !Number.isInteger(order)) throw new Error(`${position}: order must be a whole number`);
  seen.add(id);
  return { id, label, url, primary, order };
}

/**
 * Parses the list of handbooks the Naira Helm chart renders (see deploy/charts/naira/README.md)
 * and returns it with the primary handbook first, then by order, then by id.
 * Throws on the first entry that does not fit. Unknown fields are ignored.
 */
export function parseHandbooks(text: string, reservedIds: ReadonlySet<string>): Handbook[] {
  let list: unknown;
  try {
    list = JSON.parse(text);
  } catch (error) {
    throw new Error(`not valid JSON: ${error instanceof Error ? error.message : error}`);
  }
  if (!Array.isArray(list)) throw new Error('must be a list of handbooks');
  const seen = new Set<string>();
  return list
    .map((raw, i) => parseHandbook(raw, `[${i}]`, seen, reservedIds))
    .sort((a, b) => Number(b.primary) - Number(a.primary) || a.order - b.order || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/**
 * Maps handbooks to Luigi virtual tree nodes, numbered from `firstOrder` in list order.
 * Luigi appends "/:virtualSegment_1" to the node's url, so the url loses its trailing slash.
 */
export function handbookNodes(handbooks: Handbook[], category: LuigiNodeCategory, firstOrder: number): LuigiNode[] {
  return handbooks.map((handbook, i) => ({
    pathSegment: handbook.id,
    icon: 'assets/icons/handbook.svg',
    label: handbook.label,
    order: firstOrder + i,
    category,
    entityType: 'naira',
    hideFromNav: false,
    url: handbook.url.replace(/\/+$/, ''),
    virtualTree: true,
    loadingIndicator: { enabled: false },
  }));
}

/**
 * Reads the handbooks file and returns a node for each handbook. No file name means no handbooks.
 * A file that cannot be read or does not fit logs one error and yields no nodes at all.
 */
export function loadHandbookNodes(file: string | undefined, options: LoadOptions): LuigiNode[] {
  if (!file) return [];
  try {
    return handbookNodes(parseHandbooks(readFileSync(file, 'utf8'), options.reservedIds), options.category, options.firstOrder);
  } catch (error) {
    options.log(`No handbook is shown, ${file} is unusable: ${error instanceof Error ? error.message : error}`);
    return [];
  }
}
