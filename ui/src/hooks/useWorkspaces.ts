import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useId } from 'react';
import { queryKeys } from '../lib/queryKeys';
import {
  createRegistration,
  createWorkspace,
  fetchRegistrations,
  fetchWorkspaces,
  type RegistrationInput,
  updateRegistration,
} from '../lib/workspacesApi';

// Mount a fresh hook instance on authentication changes; cache keys contain no tokens.
export function useWorkspaces(token: string, workspaceId: string) {
  const scope = useId();
  const client = useQueryClient();
  const workspacesKey = queryKeys.workspaces(scope);
  const registrationsKey = queryKeys.registrations(scope, workspaceId);
  const workspaces = useQuery({
    queryKey: workspacesKey,
    queryFn: ({ signal }) => fetchWorkspaces(token, signal),
    gcTime: 0,
    retry: false,
  });
  const registrations = useQuery({
    queryKey: registrationsKey,
    queryFn: ({ signal }) => fetchRegistrations(token, workspaceId, signal),
    enabled: workspaceId !== '',
    gcTime: 0,
    retry: false,
  });
  const addWorkspace = useMutation({
    mutationFn: (name: string) => createWorkspace(token, name),
    onSuccess: () => client.invalidateQueries({ queryKey: workspacesKey }),
    gcTime: 0,
  });
  const saveRegistration = useMutation({
    mutationFn: async ({ input, id }: { input: RegistrationInput; id?: string }) => {
      if (id) {
        await updateRegistration(token, workspaceId, id, input);
      } else {
        await createRegistration(token, workspaceId, input);
      }
    },
    onSuccess: () => client.invalidateQueries({ queryKey: registrationsKey }),
    gcTime: 0,
  });
  return { workspaces, registrations, addWorkspace, saveRegistration };
}
