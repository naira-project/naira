import { useId, useState } from 'react';
import { Button } from '../components/ui/button';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { useOpenMFPContext } from '../hooks/useOpenMFPContext';
import { useWorkspaces } from '../hooks/useWorkspaces';
import type { Registration, RegistrationInput } from '../lib/workspacesApi';

export default function WorkspacesPage() {
  const { token, isReady } = useOpenMFPContext();
  if (!isReady) return <p className="p-6">Loading session…</p>;
  if (!token) return <p className="p-6">Sign in to manage your workspaces.</p>;
  return <WorkspaceSession key={token} token={token} />;
}

function WorkspaceSession({ token }: { token: string }) {
  const [workspaceId, setWorkspaceId] = useState('');
  const [workspaceName, setWorkspaceName] = useState('');
  const [editing, setEditing] = useState<Registration | null>(null);
  const [formVersion, setFormVersion] = useState(0);
  const nameId = useId();
  const { workspaces, registrations, addWorkspace, saveRegistration } = useWorkspaces(
    token,
    workspaceId,
  );
  const selected = workspaces.data?.find((workspace) => workspace.id === workspaceId);
  const changeWorkspace = (id: string) => {
    setWorkspaceId(id);
    setEditing(null);
    saveRegistration.reset();
  };

  return (
    <main className="space-y-6 p-6">
      <header>
        <h1 className="text-xl font-semibold">Workspaces</h1>
        <p className="text-sm text-muted-foreground">
          Each workspace belongs to you and can contain multiple cluster registrations.
        </p>
      </header>
      <p className="rounded border p-3 text-sm">
        Preview: data is stored in memory and lost on restart. Registration does not validate access
        or run discovery. Existing catalog views are not scoped to these workspaces.
      </p>
      <Card>
        <CardContent>
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              addWorkspace.mutate(workspaceName, {
                onSuccess: (workspace) => {
                  setWorkspaceName('');
                  changeWorkspace(workspace.id);
                },
              });
            }}
          >
            <div>
              <label htmlFor={nameId}>Workspace name</label>
              <Input
                id={nameId}
                value={workspaceName}
                onChange={(event) => setWorkspaceName(event.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={addWorkspace.isPending || saveRegistration.isPending}>
              Create workspace
            </Button>
          </form>
          {addWorkspace.error && <p role="alert">{addWorkspace.error.message}</p>}
        </CardContent>
      </Card>
      {workspaces.isPending && <p>Loading workspaces…</p>}
      {workspaces.error && <p role="alert">{workspaces.error.message}</p>}
      {workspaces.isSuccess && (
        <nav className="flex flex-wrap gap-2" aria-label="Your workspaces">
          {workspaces.data.length === 0 && <p>No workspaces yet.</p>}
          {workspaces.data.map((workspace) => (
            <Button
              key={workspace.id}
              variant={workspace.id === workspaceId ? 'default' : 'outline'}
              aria-pressed={workspace.id === workspaceId}
              disabled={saveRegistration.isPending || addWorkspace.isPending}
              onClick={() => changeWorkspace(workspace.id)}
            >
              {workspace.name}
            </Button>
          ))}
        </nav>
      )}
      {selected && workspaces.isSuccess && (
        <section className="space-y-4" aria-label="Cluster registrations">
          <h2 className="text-lg font-semibold">Clusters in {selected.name}</h2>
          {registrations.isPending && <p>Loading registrations…</p>}
          {registrations.error && <p role="alert">{registrations.error.message}</p>}
          {registrations.isSuccess && (
            <>
              {registrations.data.length === 0 && <p>No clusters registered.</p>}
              {registrations.data.map((registration) => (
                <Card key={registration.id}>
                  <CardContent className="flex items-center justify-between gap-4">
                    <div>
                      <h3 className="font-semibold">{registration.name}</h3>
                      <p>{registration.kubernetes.apiEndpoint}</p>
                      <p className="text-sm text-muted-foreground">
                        {registration.validationStatus} · {registration.id}
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      disabled={saveRegistration.isPending}
                      onClick={() => {
                        setEditing(registration);
                        saveRegistration.reset();
                      }}
                    >
                      Edit {registration.name}
                    </Button>
                  </CardContent>
                </Card>
              ))}
              <RegistrationForm
                key={`${workspaceId}:${editing?.id ?? 'new'}:${formVersion}`}
                registration={editing}
                pending={saveRegistration.isPending}
                error={saveRegistration.error?.message}
                onCancel={() => {
                  setEditing(null);
                  saveRegistration.reset();
                }}
                onSave={(input) => {
                  saveRegistration.mutate(
                    { input, id: editing?.id },
                    {
                      onSuccess: () => {
                        setEditing(null);
                        setFormVersion((version) => version + 1);
                        saveRegistration.reset();
                      },
                    },
                  );
                }}
              />
            </>
          )}
        </section>
      )}
    </main>
  );
}

function RegistrationForm({
  registration,
  pending,
  error,
  onSave,
  onCancel,
}: {
  registration: Registration | null;
  pending: boolean;
  error?: string;
  onSave: (input: RegistrationInput) => void;
  onCancel: () => void;
}) {
  const id = useId();
  const [name, setName] = useState(registration?.name ?? '');
  const [endpoint, setEndpoint] = useState(registration?.kubernetes.apiEndpoint ?? '');
  const [context, setContext] = useState(registration?.kubernetes.context ?? '');
  const [credentialRef, setCredentialRef] = useState('');
  return (
    <Card>
      <CardContent>
        <h3 className="mb-3 font-semibold">
          {registration ? 'Edit registration' : 'Register Kubernetes cluster'}
        </h3>
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            onSave({ name, credentialRef, kubernetes: { apiEndpoint: endpoint, context } });
          }}
        >
          <label className="block" htmlFor={`${id}-name`}>
            Cluster name
            <Input
              id={`${id}-name`}
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </label>
          <label className="block" htmlFor={`${id}-endpoint`}>
            Kubernetes API endpoint
            <Input
              id={`${id}-endpoint`}
              type="url"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              required
            />
          </label>
          <label className="block" htmlFor={`${id}-context`}>
            Kubeconfig context (optional)
            <Input
              id={`${id}-context`}
              value={context}
              onChange={(e) => setContext(e.target.value)}
            />
          </label>
          <label className="block" htmlFor={`${id}-reference`}>
            Credential reference — not the credential value
            <Input
              id={`${id}-reference`}
              type="password"
              autoComplete="off"
              value={credentialRef}
              onChange={(e) => setCredentialRef(e.target.value)}
              required
            />
          </label>
          <p className="text-sm text-muted-foreground">
            References are not resolved or returned by the API. When editing, re-enter the
            reference. Do not paste a kubeconfig or token.
          </p>
          {error && <p role="alert">{error}</p>}
          <Button type="submit" disabled={pending}>
            {registration ? 'Save registration' : 'Register cluster'}
          </Button>
          {registration && (
            <Button type="button" variant="outline" onClick={onCancel} disabled={pending}>
              Cancel
            </Button>
          )}
        </form>
      </CardContent>
    </Card>
  );
}
