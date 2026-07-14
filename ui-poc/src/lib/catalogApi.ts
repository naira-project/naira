export interface PluginClaim {
	plugin: string;
	props: Record<string, string>;
}

export interface NodeResource {
	name: string;
	kind: string;
	path: string;
	pluginClaims: PluginClaim[];
}

/**
 * Merge all plugin claim props into a single flat map.
 * Later plugins override earlier ones for the same key.
 */
// TODO(#104): Visualise props from all plugins in the properties panel, instead of overwriting them by key.
export function nodeProps(node: NodeResource): Record<string, string> {
	const merged: Record<string, string> = {};
	for (const claim of node.pluginClaims ?? []) {
		for (const [key, value] of Object.entries(claim.props)) {
			merged[key] = value;
		}
	}
	return merged;
}

export interface NodeNameParts {
	kind: string;
	path: string;
}

interface ListNodesResponse {
	nodes?: NodeResource[];
}

export interface RelationResource {
	name: string;
	kind: string;
	fromNode: string;
	toNode: string;
}

interface ListRelationsResponse {
	relations?: RelationResource[];
}

interface ListRequest {
	filter?: string;
	pageSize?: number;
	pageToken?: string;
}

function buildListUrl(basePath: string, options: ListRequest = {}) {
	const params = new URLSearchParams();

	if (options.filter) {
		params.set('filter', options.filter);
	}
	if (typeof options.pageSize === 'number') {
		params.set('pageSize', String(options.pageSize));
	}
	if (options.pageToken) {
		params.set('pageToken', options.pageToken);
	}

	const query = params.toString();
	return query ? `${basePath}?${query}` : basePath;
}

export function encodeCatalogPath(path: string) {
	return path
		.split('/')
		.map((segment) => encodeURIComponent(segment))
		.join('/');
}

export function buildEqualityFilter(field: string, value: string) {
	return `${field}="${value}"`;
}

export function buildNodeUrl(kind: string, path: string) {
	return `/v1/nodes/${encodeURIComponent(kind)}/${encodeCatalogPath(path)}`;
}

export function parseNodeName(name: string): NodeNameParts | null {
	const parts = name.split('/');
	if (parts.length < 3 || parts[0] !== 'nodes') {
		return null;
	}

	const kind = parts[1] ?? '';
	const path = parts.slice(2).join('/');
	if (!kind || !path) {
		return null;
	}

	return { kind, path };
}

export function buildListNodesUrl(options: ListRequest = {}) {
	return buildListUrl('/v1/nodes', options);
}

export function buildListRelationsUrl(options: ListRequest = {}) {
	return buildListUrl('/v1/relations', options);
}

async function fetchJson<T>(url: string) {
	const response = await fetch(url);
	if (!response.ok) {
		throw new Error(`Request failed for ${url}`);
	}

	return response.json() as Promise<T>;
}

export async function fetchNodes(options: ListRequest = {}) {
	const data = await fetchJson<ListNodesResponse>(buildListNodesUrl(options));
	return data.nodes ?? [];
}

export async function fetchNode(kind: string, path: string) {
	return fetchJson<NodeResource>(buildNodeUrl(kind, path));
}

export async function fetchNodeByName(name: string) {
	const node = parseNodeName(name);
	if (!node) {
		throw new Error(`Invalid node name: ${name}`);
	}

	return fetchNode(node.kind, node.path);
}

export async function fetchRelations(options: ListRequest = {}) {
	const data = await fetchJson<ListRelationsResponse>(buildListRelationsUrl(options));
	return data.relations ?? [];
}