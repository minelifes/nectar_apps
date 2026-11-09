# Helm charts

Project page → **Helm charts** → *Install chart*: any Helm chart, from a chart repository
(`https://grafana.github.io/helm-charts`) or an OCI registry (`oci://registry-1.docker.io/bitnamicharts`),
with a version (empty = latest) and your values (YAML). Private repositories take a user and a password or
token.

## How it runs

- Each operation (install, upgrade, rollback, uninstall) is a short Job in the project's namespace running
  the `helm` CLI (`alpine/helm`). Its ServiceAccount `nectar-helm` may do anything **inside that namespace and
  nothing outside it**: a chart can't touch other projects or the cluster. Charts that install CRDs or
  cluster-wide roles therefore fail with Helm's "forbidden" message — install those with `helm` as a cluster
  admin instead.
- Upgrades use `--atomic`: a failed upgrade goes back to the previous revision by itself; a failed first
  install leaves nothing behind.
- The project's quota and Pod Security level (baseline) apply to the chart's pods.
- Values and the repository password are stored encrypted with the panel's master key (rotated with it).
  Only people with **helm.manage** (Admin and Developer roles) see the values and change charts; anyone who
  sees the project sees the releases, their history and status.
- The history, status and notes come from Helm itself (its release Secrets), so a release changed with the
  `helm` CLI shows the same.

## Domains for a chart

The chart's Services are listed on its page as `name.namespace.svc.cluster.local:port`. To put a domain on
one, add a proxy (Domains → Proxies) with that address — HTTPS, rate limits and the other domain options work
as usual. Charts that create Ingress objects are picked up by the panel's Ingress sync too.

## Notes

- Uninstalling removes what the chart created; whether its volumes (PVCs) go too depends on the chart.
- Workloads of panel-installed charts are not offered for import in adopted projects.
- API: `GET/POST /api/projects/{id}/helm`, `GET/PUT/DELETE …/helm/{name}`, `POST …/helm/{name}/rollback`.
