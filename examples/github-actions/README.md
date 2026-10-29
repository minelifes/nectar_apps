# Deploy from GitHub Actions

1. In the panel: **Settings → API tokens → New token**. Pick the role **Deployer** and the project the app
   lives in (the token can then only view, deploy and roll back apps of that project). Copy the token.
2. In the GitHub repository: **Settings → Secrets and variables → Actions**: add `NECTAR_URL`
   (e.g. `https://panel.apps.example.com`) and `NECTAR_TOKEN`.
3. Copy [`deploy-jar.yml`](deploy-jar.yml) or [`deploy-image.yml`](deploy-image.yml) to `.github/workflows/`
   and change `app:` to your `project/app`.

The action downloads the `nectar` CLI from the release, checks its SHA-256, uploads the file (or switches the
image), starts the release and waits until it is live. A failed release fails the job; the app keeps serving
the previous release. The release number is available as `steps.<id>.outputs.release`.

The panel must be reachable from GitHub's runners (published through the gateway, e.g. `panel.<base domain>`).
If it is not, run the same `nectar deploy` from a self-hosted runner.
