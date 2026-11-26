import { useId, useState } from 'react';
import { Button } from '../components/ui/button';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { useOpenMFPContext } from '../hooks/useOpenMFPContext';
import { useTestWorkspace } from '../hooks/useTestWorkspace';
import type { Registration, RegistrationInput } from '../lib/workspacesApi';

export default function TestWorkspacePage() {
  const { token, isReady } = useOpenMFPContext();
  if (!isReady) return <p className="p-6">Loading session…</p>;
  if (!token) return <p className="p-6">Sign in to manage clusters in test workspace.</p>;
  return <WorkspaceSession key={token} token={token} />;
}

function WorkspaceSession({ token }: { token: string }) {
  const [editing, setEditing] = useState<Registration | null>(null);
  const [formVersion, setFormVersion] = useState(0);
  const { registrations, saveRegistration, testConnection } = useTestWorkspace(token);

  return (
    <main className="space-y-6 p-6">
      <header>
        <h1 className="text-xl font-semibold">test workspace</h1>
        <p className="text-sm text-muted-foreground">
          Add and edit clusters in test-workspace, shared by all signed-in users.
        </p>
      </header>
      <p className="rounded border p-3 text-sm">
        Preview: data is stored in memory and lost on restart. Use Test connection after saving.
        Tests require an operator-approved credential and do not import resources or run discovery.
        Existing catalog views are not scoped to this workspace.
      </p>
      <section className="space-y-4" aria-label="Cluster registrations">
        <h2 className="text-lg font-semibold">Clusters</h2>
        {registrations.isPending && <p>Loading registrations…</p>}
        {registrations.error && <p role="alert">{registrations.error.message}</p>}
        {registrations.isSuccess && (
          <>
            {registrations.data.length === 0 && <p>No clusters registered.</p>}
            {registrations.data.map((registration) => (
              <Card key={registration.id}>
                <CardContent className="space-y-3">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <h3 className="font-semibold">{registration.name}</h3>
                      <p>{registration.kubernetes.apiEndpoint}</p>
                      <p className="text-sm text-muted-foreground">
                        {registration.validationStatus} · {registration.id}
                      </p>
                    </div>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        disabled={saveRegistration.isPending || testConnection.isPending}
                        onClick={() => testConnection.mutate(registration.id)}
                        aria-label={`Test connection ${registration.name}`}
                      >
                        {testConnection.isPending && testConnection.variables === registration.id
                          ? 'Testing…'
                          : 'Test connection'}
                      </Button>
                      <Button
                        variant="outline"
                        disabled={saveRegistration.isPending || testConnection.isPending}
                        onClick={() => {
                          setEditing(registration);
                          saveRegistration.reset();
                        }}
                      >
                        Edit {registration.name}
                      </Button>
                    </div>
                  </div>
                  {testConnection.error && testConnection.variables === registration.id && (
                    <p role="alert">
                      Could not save the connection test result. {testConnection.error.message}{' '}
                      Retry if the configuration changed.
                    </p>
                  )}
                  {registration.lastValidation && (
                    <section
                      aria-label={`Connection results ${registration.name}`}
                      className="rounded border p-3 text-sm"
                    >
                      <p>
                        Last connection test: {registration.lastValidation.status} ·{' '}
                        {new Date(registration.lastValidation.checkedAt).toLocaleString()}
                      </p>
                      <ul className="mt-2 space-y-1">
                        {registration.lastValidation.checks.map((check) => (
                          <li key={check.name}>
                            <strong>
                              {check.passed ? 'PASS' : 'FAIL'} — {check.name}
                            </strong>
                            : {check.message} ({check.code})
                          </li>
                        ))}
                      </ul>
                    </section>
                  )}
                </CardContent>
              </Card>
            ))}
            <RegistrationForm
              key={`${editing?.id ?? 'new'}:${formVersion}`}
              registration={editing}
              pending={saveRegistration.isPending || testConnection.isPending}
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
            Use an operator-approved kubeconfig-file:/absolute/path or
            kubernetes-secret:namespace/name#key. References are not returned by the API; re-enter
            the reference when editing. Do not paste a kubeconfig or token.
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
