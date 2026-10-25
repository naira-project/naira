export interface Workspace {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
}

export interface KubernetesConnection {
  apiEndpoint: string;
  context?: string;
}

export interface Registration {
  id: string;
  workspaceId: string;
  name: string;
  kind: 'kubernetes';
  kubernetes: KubernetesConnection;
  kubeSystemUid?: string;
  validationStatus: 'PENDING' | 'READY' | 'FAILED';
  lastValidationAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface RegistrationInput {
  name: string;
  credentialRef: string;
  kubernetes: KubernetesConnection;
}

async function request(
  token: string,
  path: string,
  method = 'GET',
  body?: unknown,
  signal?: AbortSignal,
): Promise<Response> {
  const response = await fetch(path, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
    cache: 'no-store',
  });
  if (!response.ok) {
    const messages: Record<number, string> = {
      400: 'Check the name, endpoint and credential reference.',
      401: 'Sign in again to continue.',
      403: 'You do not have access to this workspace.',
      404: 'Registration not found in this workspace.',
      409: 'That name is already in use.',
    };
    throw new Error(messages[response.status] ?? 'The request failed. Please try again.');
  }
  return response;
}

function registrationsPath(workspaceId: string) {
  return `/v1/workspaces/${encodeURIComponent(workspaceId)}/registrations`;
}

export async function fetchWorkspaces(token: string, signal?: AbortSignal): Promise<Workspace[]> {
  const response = await request(token, '/v1/workspaces', 'GET', undefined, signal);
  const data = (await response.json()) as { workspaces: Workspace[] };
  return data.workspaces;
}

export async function createWorkspace(token: string, name: string): Promise<Workspace> {
  const response = await request(token, '/v1/workspaces', 'POST', { name });
  return response.json();
}

export async function fetchRegistrations(
  token: string,
  workspaceId: string,
  signal?: AbortSignal,
): Promise<Registration[]> {
  const response = await request(token, registrationsPath(workspaceId), 'GET', undefined, signal);
  const data = (await response.json()) as { registrations: Registration[] };
  return data.registrations;
}

export async function createRegistration(
  token: string,
  workspaceId: string,
  input: RegistrationInput,
): Promise<Registration> {
  const response = await request(token, registrationsPath(workspaceId), 'POST', {
    ...input,
    kind: 'kubernetes',
  });
  return response.json();
}

export async function updateRegistration(
  token: string,
  workspaceId: string,
  id: string,
  input: RegistrationInput,
): Promise<void> {
  await request(token, `${registrationsPath(workspaceId)}/${encodeURIComponent(id)}`, 'PUT', input);
}
