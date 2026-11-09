# On-call guide

One engineer of the Platform team is on call at any time. This page says what that means, how to
rate an incident and what to do next. It is long on purpose: use the headings to jump to a part.

## The rota

On-call changes every Monday at 09:00 Central European Time. The outgoing engineer hands over in
the `#platform-incidents` channel and lists open incidents, flaky alerts and anything unusual.

Swaps are your own to arrange. Post the swap in the channel so the rota stays true.

## Severity levels

Rate an incident by what users cannot do, not by how much is broken inside.

| Level | Meaning | Example | First response | Update every |
|---|---|---|---|---|
| SEV1 | Users cannot use a core feature at all | The model gateway returns errors for all requests | 15 minutes | 30 minutes |
| SEV2 | A core feature is degraded for many users | Requests are slow, or one provider fails | 30 minutes | 1 hour |
| SEV3 | A small group or a minor feature is affected | One team's CI queue is stuck | 4 working hours | Daily |

When in doubt, rate one level higher. You can lower it later.

## When you are paged

1. **Acknowledge** the page within 5 minutes, so the system does not escalate to the next person.
2. **Look** at the dashboard of the affected service. Decide what the users cannot do.
3. **Rate** it with the table above and write the level in `#platform-incidents`.
4. **Fix or mitigate.** Prefer the action that restores service fastest, even if it is not the final
   fix. The [runbooks](/runbooks/restart-the-model-gateway) cover the common cases.
5. **Tell people.** Post an update at the interval of the level, even if the update is "no change".
6. **Close.** Say that service is restored and when.

## Escalate

Escalate when you are stuck for 30 minutes on a SEV1 or SEV2, or when you need access you do not
have. Page the secondary on-call first, then the team lead.

::: danger Do not wait
An escalation is not a failure. A SEV1 that waits for one person to try everything costs more than
an extra person who was not needed.
:::

## After the incident

Within 3 working days, write a short review: what happened, how it was found, what fixed it and
what we change. Keep it blameless. If the review changes how we work, add an entry to the
[decision log](/decisions/).

## Tools and access

| What | Why | How to get it |
|---|---|---|
| The `platform-prod` cluster, with write access during your shift | To restart, scale or roll back | Granted automatically at the start of your shift |
| The paging tool | To receive and acknowledge pages | Your onboarding includes an account |
| The model gateway dashboard | To see request and error rates | Open to the whole company |
