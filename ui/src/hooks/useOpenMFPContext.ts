import * as LuigiClient from '@luigi-project/client';
import { useSyncExternalStore } from 'react';

export interface OpenMFPContext {
  token: string | null;
  userId: string | null;
  userEmail: string | null;
  isReady: boolean;
}

const READY_TIMEOUT_MS = 500;

let state: OpenMFPContext = {
  token: null,
  userId: null,
  userEmail: null,
  isReady: false,
};

const subscribers = new Set<() => void>();
let started = false;

function notify() {
  subscribers.forEach((listener) => {
    listener();
  });
}

function stringOrNull(value: unknown): string | null {
  return typeof value === 'string' ? value : null;
}

function update(context: Record<string, unknown>) {
  state = {
    token: stringOrNull(context.token),
    userId: stringOrNull(context.userId),
    userEmail: stringOrNull(context.userEmail),
    isReady: true,
  };
  notify();
}

// Luigi's listeners are process-wide, so they must be registered exactly once
// regardless of how many components subscribe to this store.
function ensureStarted() {
  if (started) {
    return;
  }
  started = true;

  LuigiClient.addInitListener(update);
  LuigiClient.addContextUpdateListener(update);

  setTimeout(() => {
    if (!state.isReady) {
      state = { ...state, isReady: true };
      notify();
    }
  }, READY_TIMEOUT_MS);
}

function subscribe(listener: () => void): () => void {
  ensureStarted();
  subscribers.add(listener);
  return () => {
    subscribers.delete(listener);
  };
}

function getSnapshot(): OpenMFPContext {
  return state;
}

export function useOpenMFPContext(): OpenMFPContext {
  return useSyncExternalStore(subscribe, getSnapshot);
}
