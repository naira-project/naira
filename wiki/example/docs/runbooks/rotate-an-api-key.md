# Rotate an API key

Rotate a provider key when it leaks, when someone with access leaves, or every 90 days. The gateway
holds the keys, so product teams do not change anything.

## Before you start

You need write access to the secret store and the Platform team's role in the provider's console.
Rotate during working hours when you can. Rotation does not interrupt service if you follow the
order below.

## Steps

1. **Create the new key** in the provider's console. Name it with today's date, for example
   `gateway-2026-11-07`.
2. **Store it** next to the old one, under a new version:

   ::: code-group

   ```sh [Secret store CLI]
   secretctl put platform/model-gateway/provider-key --version next < new-key.txt
   ```

   ```yaml [Declarative]
   apiVersion: example.com/v1
   kind: SecretVersion
   metadata:
     name: provider-key-next
   spec:
     path: platform/model-gateway/provider-key
   ```

   :::

3. **Switch the gateway** to the new version and restart it, see
   [Restart the model gateway](/runbooks/restart-the-model-gateway).
4. **Check** on the dashboard that requests to that provider succeed.
5. **Revoke the old key** in the provider's console, after the check passes.
6. **Record it** in the rotation log with the date and the reason.

## If something goes wrong

If requests to the provider fail after the switch, put the old key back as the current version and
restart the gateway again. Only then investigate. Do not revoke the old key until the new one works.

## Why this order

The old key stays valid until the new one is proven. That way a mistake costs a second restart,
not an outage.
