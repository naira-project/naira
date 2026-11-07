# Onboarding

What to do in the first week on the Platform team, day by day.

## Day 1: access

- Get an account in the identity provider and join the group `platform`.
- Join the `#platform` and `#platform-incidents` channels.
- Ask your buddy for access to the `platform-prod` and `platform-ci` clusters, read-only.
- Read [About the team](/team/about) to know what we run.

## Day 2 and 3: see it work

- Open the dashboard of the model gateway and find the request rate and the error rate.
- Send a test request through the gateway from your laptop.
- Read the two runbooks: [restart the model gateway](/runbooks/restart-the-model-gateway) and
  [rotate an API key](/runbooks/rotate-an-api-key).

## Day 4: shadow on-call

- Read the [on-call guide](/oncall/guide), especially the severity levels.
- Sit in on a handover with the outgoing on-call engineer.
- Read the [decision log](/decisions/) to learn why things are the way they are.

## Day 5: first change

- Fix a typo or an unclear sentence in this handbook and open a pull request.
- Ask your buddy to review it, and merge it once it builds.

::: tip Why start with this handbook
A change to a page is small, has a reviewer and shows the whole path from edit to published page.
The build fails on a broken link, so you also learn how links work here.
:::
