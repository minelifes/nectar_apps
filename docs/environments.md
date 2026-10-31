# Environments and env groups

## Env groups

A project's **env groups** (project page → Env groups) hold variables several apps share — a region, an API
endpoint, a Sentry DSN. Apps pick their groups on the Environment tab (their own variables win); anywhere else,
reference a value as `${group.KEY}`. Changing a group marks its apps for redeploy.

## Copies and environments

- **Copy app** (app menu): settings, variables (secrets included), files, groups and links into a new app of
  the same project. It is not deployed until you deploy it.
- **New environment** (project menu): clones the whole project — env groups, new empty databases with new
  passwords, and the apps — as e.g. `shop-staging`, labelled with its environment and linked to its origin.
- **Promote** (app menu): deploys the live image or artifact of an app to the app with the same slug in the
  other environment (staging → production) without rebuilding.
