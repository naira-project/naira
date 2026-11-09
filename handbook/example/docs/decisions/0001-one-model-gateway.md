# 0001 One gateway for all models

| | |
|---|---|
| Status | Accepted |
| Date | 2026-03-12 |
| Deciders | Platform team |

## Context

Four product teams call model providers. Each holds its own keys, writes its own retry code and
sees only its own usage. When a provider changed a price, nobody could say what it cost us. When a
key leaked, we could not tell who used it.

## Options

### A. Every team calls providers directly

- Good: no shared component to run.
- Bad: keys in four places, four sets of retry code, no total view of cost.

### B. One shared gateway that routes requests to providers

- Good: keys in one place, one place to limit and measure usage, a provider can change without a
  change in any product.
- Bad: one more service to run, and a failure in it affects every team.

### C. A library that each team includes

- Good: no extra service.
- Bad: the keys still live in each product, and a fix needs four releases.

## Decision

We run option B. Product teams call the gateway at its internal address and never hold a provider
key.

## Consequences

- We own the availability of the gateway. It runs with 3 replicas, and a runbook covers
  [restarting it](/runbooks/restart-the-model-gateway).
- Keys are rotated in one place, see [Rotate an API key](/runbooks/rotate-an-api-key).
- A team that needs a feature the gateway lacks asks us. It does not work around it.

## Revisit when

A single provider serves nearly all traffic for a year, or the gateway causes more than one SEV1 a
quarter. Then compare option A again.
