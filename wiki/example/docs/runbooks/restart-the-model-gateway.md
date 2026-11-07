# Restart the model gateway

Use this when the model gateway returns errors for all requests, or stops answering, and a restart
is the fastest way to restore service. A restart takes about 2 minutes and drops requests that are
in flight.

## Before you start

- Confirm it is the gateway and not a provider. Open the gateway dashboard. If only one provider's
  requests fail, do not restart: remove that provider from the routing instead.
- Post in `#platform-incidents` that you are restarting the gateway.

## Restart

1. Check the current state:

   ```sh
   kubectl -n platform get deploy model-gateway
   ```

   The `READY` column shows how many of the replicas answer.

2. Start a rolling restart. It replaces the pods one at a time, so the gateway stays up while it
   runs:

   ```sh
   kubectl -n platform rollout restart deploy/model-gateway
   ```

3. Wait until it finishes:

   ```sh
   kubectl -n platform rollout status deploy/model-gateway --timeout=5m
   ```

   You should see `successfully rolled out`.

4. Check on the dashboard that the error rate drops within 2 minutes.

## If it does not recover

::: details The rollout stays stuck
Look at the pods. A pod that cannot start usually says why:

```sh
kubectl -n platform describe pod -l app=model-gateway
```

If the new version fails, roll back to the previous one:

```sh
kubectl -n platform rollout undo deploy/model-gateway
```
:::

::: details Errors continue after a clean restart
The cause is outside the gateway. Check the status pages of the providers, and the vector store.
Rate the incident again and [escalate](/oncall/guide#escalate) if it is a SEV1.
:::

## Afterwards

Write what you did in the incident thread. If you needed this runbook twice in a month, open a
pull request to fix the cause, not only this page.
