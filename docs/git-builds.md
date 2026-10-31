# Git builds and registries

An app of the **Docker image** type can be built from a Git repository (New app → Docker image → Build from
Git, or the app's Settings): the repository URL, the branch and, for private repositories, an access token.
The repository needs a `Dockerfile`.

- Each build is a [Kaniko](https://github.com/GoogleContainerTools/kaniko) Job in the project's namespace. It
  pushes `registry.nectar.local/<project>/<app>:build-N` to the **built-in registry** and deploys it. The
  Builds tab shows every build with its log; the newest 5 images per app are kept.
- **Push to deploy:** add the webhook shown on the Builds tab to GitHub, GitLab or Gitea; every push to the
  branch builds and deploys.
- Kaniko v1.23.2 is the default builder; set `NECTAR_BUILDER_IMAGE` on the panel to use a maintained fork:
  `kubectl -n nectar-system set env deploy/nectar-apps NECTAR_BUILDER_IMAGE=<image>`.

## Preview environments

Turn on **A preview for every pull request** in the app's Git source (Builds tab → Edit) and add pull request
events to the webhook (GitHub/Gitea: *Pull requests*; GitLab: *Merge request events*). Then every pull request
into the app's branch gets a preview:

- an app `<app>-pr<N>` in the same project — a copy of the app's settings, variables, config files and env groups,
  with one replica — at `https://<app>-pr<N>-<project>.<base domain>`;
- built from the pull request's head (`refs/pull/N/head`, GitLab `refs/merge-requests/N/head`), so pull requests
  from forks work too when you allow them;
- rebuilt on every new commit, and deleted when the pull request is closed or merged, or after 14 days without
  activity (configurable, or never);
- listed on the parent app's Builds tab with links to the preview and the pull request.

**Databases:** previews start without database links, so they cannot change real data. Allow
*Previews use this app's databases* only when that is safe (e.g. a staging project).
**Forks:** pull requests from forks are skipped unless allowed — their code would run with the app's variables
and secrets. At most 10 previews per app run at a time.

**Commit status:** when the source's access token may write commit statuses (GitHub fine-grained token:
*Commit statuses: write*; Gitea: *write:repository*; GitLab: *api*), the pull request shows `nectar/preview` —
pending while it builds, then success with a link to the preview, or failure with a link to the build log.

## The built-in registry

`install.sh` sets it up on new servers (`--registry` adds it to an existing one and restarts k3s once;
`--no-registry` skips it). It is only reachable from the cluster and needs a password; nodes pull through a
k3s mirror (`/etc/rancher/k3s/registries.yaml`). The installer manages only its own block of that file; if the
file already has `mirrors:` or `configs:` of yours, it leaves it alone and prints the entries to merge.

## Private registries

Images from GHCR, Docker Hub, GitLab or your own registry work with a login under **Settings → Registries**:
every project gets it as an image pull secret, and builds use it for their base images.
