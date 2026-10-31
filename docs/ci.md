# CI / GitHub Actions

Any CI can deploy with the [CLI](cli.md) and two secrets, `NECTAR_URL` and `NECTAR_TOKEN` (a **Deployer**
token limited to the project). For GitHub Actions there is an action:

```yaml
- uses: minelifes/nectar_apps@v0
  with:
    url: ${{ secrets.NECTAR_URL }}
    token: ${{ secrets.NECTAR_TOKEN }}
    app: shop/api
    file: target/app.jar          # or: image: ghcr.io/acme/api:${{ github.sha }}
```

Complete workflows (build a JAR and deploy it, build and push an image and deploy it) are in
[`examples/github-actions`](https://github.com/minelifes/nectar_apps/tree/main/examples/github-actions).

Alternatively let the panel build: [Git builds](git-builds.md) with a push webhook need no CI at all.

## Over HTTP

Everything the panel does is available over HTTP with `Authorization: Bearer nat_…` (see the
[API reference](api.md)). Large files can use the resumable [tus](https://tus.io) endpoint
`POST /api/apps/{id}/uploads`.
