import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useId } from 'react';
import { queryKeys } from '../lib/queryKeys';
import {
  createRegistration,
  fetchRegistrations,
  type RegistrationInput,
  TEST_WORKSPACE_ID,
  testRegistration,
  updateRegistration,
} from '../lib/workspacesApi';

// Mount a fresh hook instance on authentication changes; cache keys contain no tokens.
export function useTestWorkspace(token: string) {
  const scope = useId();
  const client = useQueryClient();
  const registrationsKey = queryKeys.registrations(scope, TEST_WORKSPACE_ID);
  const registrations = useQuery({
    queryKey: registrationsKey,
    queryFn: ({ signal }) => fetchRegistrations(token, signal),
    gcTime: 0,
    retry: false,
  });
  const saveRegistration = useMutation({
    mutationFn: async ({ input, id }: { input: RegistrationInput; id?: string }) => {
      if (id) {
        await updateRegistration(token, id, input);
      } else {
        await createRegistration(token, input);
      }
    },
    onSuccess: () => client.invalidateQueries({ queryKey: registrationsKey }),
    gcTime: 0,
  });
  const testConnection = useMutation({
    mutationFn: (id: string) => testRegistration(token, id),
    onSettled: () => client.invalidateQueries({ queryKey: registrationsKey }),
    gcTime: 0,
  });
  return { registrations, saveRegistration, testConnection };
}
