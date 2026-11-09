# What you can write

This page shows what a handbook built from the starter can render. Each section is a pattern you can
copy. Open the page source next to the result to see how it is written.

## Headings and links

A page starts with one `#` heading. Use `##` and `###` below it. Each heading is a place that you
can link to, for example [the severity levels](/oncall/guide#severity-levels).

Link to other pages with an absolute path that starts at the root of the handbook, such as
`/team/about`. A link to a page that does not exist fails the build, so a rename cannot go
unnoticed.

## Tables

| Service | Owner | Tier |
|---|---|---|
| Model gateway | Platform | 1 |
| Vector store | Platform | 2 |
| Build runners | Platform | 2 |

## Code

Inline code looks like `kubectl get pods`. A block names its language and gets highlighting:

```yaml
replicas: 3
resources:
  limits:
    memory: 512Mi
```

Mark lines to draw the eye:

```sh{2}
kubectl -n platform get deploy
kubectl -n platform rollout restart deploy/model-gateway
```

Group alternatives in tabs:

::: code-group

```sh [macOS]
brew install kubectl
```

```sh [Linux]
sudo apt-get install -y kubectl
```

:::

## Containers

::: info
Use **info** for background the reader may want.
:::

::: tip
Use **tip** for advice that saves time.
:::

::: warning
Use **warning** for something that can cause trouble.
:::

::: danger
Use **danger** for something that can cause an outage or data loss.
:::

::: details Use details to fold long text away
The reader opens it only when they need it. Good for logs, long output and edge cases.
:::

## Badges

A status next to a title, for example <Badge type="tip" text="accepted" /> or
<Badge type="warning" text="proposed" />.

## Lists

1. Numbered lists for steps in a fixed order.
2. Bullets for things that have no order.

A task list, `- [ ] item`, is not rendered as a checkbox: it shows as a plain bullet with the
brackets. Write a plain list instead.

## Search

The search box finds words in headings and text on every page of this handbook. Try `rollout` or
`severity`. A result opens its page and scrolls to the heading.
