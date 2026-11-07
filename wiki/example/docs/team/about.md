# About the team

The Platform team at Example Corp runs the shared infrastructure that product teams build on. This
page says what we run, who to ask and when we are reachable.

## What we run

| Service | What it does | Where it runs |
|---|---|---|
| Model gateway | One internal endpoint that routes requests to model providers | `platform-prod` cluster |
| Vector store | Shared similarity search for retrieval features | `platform-prod` cluster |
| Build runners | Runs the CI jobs of every product team | `platform-ci` cluster |

Product teams use the model gateway through its internal address. They never call a provider
directly, so we can change providers without a change on their side. The reasoning is in
[0001 One gateway for all models](/decisions/0001-one-model-gateway).

## How to reach us

| For | Use | Answer within |
|---|---|---|
| Something is broken now | Page the on-call, see the [on-call guide](/oncall/guide) | 15 minutes |
| A question or a request | The `#platform` channel | 1 working day |
| A change to how we work | A pull request on this handbook | 5 working days |

## Working hours

We work Monday to Friday, 08:00 to 17:00 Central European Time. Outside those hours only the
on-call engineer is reachable, and only for incidents.

## Joining the team

New members follow the [onboarding checklist](/team/onboarding).
