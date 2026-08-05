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

// ---------------------------------------------------------------------------
// Plugin discovery
// ---------------------------------------------------------------------------

interface ListPluginsResponse {
	plugins: string[];
}

/**
 * GET /v1/plugins — returns the list of registered plugin names.
 */
export async function fetchPlugins(): Promise<string[]> {
	const data = await fetchJson<ListPluginsResponse>('/v1/plugins');
	return data.plugins ?? [];
}

// ---------------------------------------------------------------------------
// Plugin run operations (AIP-151)
// ---------------------------------------------------------------------------

export interface StatusErrorResource {
	message: string;
}

export interface OperationResource {
	name: string;
	plugin: string;
	state: 'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'FAILED';
	startTime: string;
	endTime?: string;
	error?: StatusErrorResource;
	nodesUpserted: number;
	relationsUpserted: number;
	createdAt: string;
}

interface RunPluginsResponse {
	operations: OperationResource[];
}

interface ListOperationsResponse {
	operations?: OperationResource[];
}

export function buildListOperationsUrl(options: ListRequest = {}) {
	return buildListUrl('/v1/operations', options);
}

/**
 * POST /v1/plugins:run — triggers all registered plugins asynchronously.
 * Returns the list of tracking operations.
 */
export async function runAllPlugins(): Promise<OperationResource[]> {
	const response = await fetch('/v1/plugins:run', { method: 'POST' });
	if (!response.ok) {
		const payload = await response.json().catch(() => ({}));
		throw new Error(payload.error ?? 'Failed to trigger plugin run');
	}

	const payload = await response.json() as RunPluginsResponse;
	return payload.operations ?? [];
}

/**
 * POST /v1/{plugin}:run — triggers a single plugin asynchronously.
 * Returns the tracking operation.
 */
export async function runPlugin(plugin: string): Promise<OperationResource> {
	const response = await fetch(`/v1/${encodeURIComponent(plugin)}:run`, { method: 'POST' });
	if (!response.ok) {
		const payload = await response.json().catch(() => ({}));
		throw new Error(payload.error ?? `Failed to trigger plugin "${plugin}"`);
	}

	return response.json() as Promise<OperationResource>;
}

/**
 * GET /v1/operations/{operationId} — fetches a single operation by name.
 */
export async function fetchOperation(operationId: string): Promise<OperationResource> {
	return fetchJson<OperationResource>(`/v1/operations/${encodeURIComponent(operationId)}`);
}

/**
 * GET /v1/operations — lists operations with optional filter.
 */
export async function fetchOperations(options: ListRequest = {}): Promise<OperationResource[]> {
	const data = await fetchJson<ListOperationsResponse>(buildListOperationsUrl(options));
	return data.operations ?? [];
}