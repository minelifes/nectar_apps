# Existing k3s server (adopt mode)

Nectar can move into a k3s server that already runs apps — without breaking them on the way.

```bash
sudo sh install.sh --discover          # read-only report: proxies, routes, TLS, and what migrates (✓ ⚠ ✗)
sudo sh install.sh --adopt             # backup → shadow install → verify → cutover (asks before each change)
sudo sh install.sh --status            # phase, plus hosts that still reach the old proxy
sudo sh install.sh --rollback          # the old proxy serves 80/443 again
sudo sh install.sh --finalize          # once nothing uses the old proxy any more
```

## Discovery

The report lists in-cluster controllers (Traefik, ingress-nginx, HAProxy, Kong, Contour, Istio, Cilium),
proxies on the host (nginx, Caddy, Apache, HAProxy, a standalone nectar_gateway), who holds ports 80/443 and
every route with its TLS setup, marking what migrates as is (✓), with caveats (⚠) or not at all (✗).

## Strategies

- **migrate** (default): nectar-gateway takes 80/443; the old proxy stays behind it as a fallback for anything
  it could not translate.
- **keep**: your ingress controller stays; the panel writes standard Ingress objects for its apps.
- **adopt-gateway**: import a standalone nectar_gateway.

## Safety

- The backup covers the k3s datastore, routing objects and proxy configs.
- The new gateway first runs on shadow ports 18080/18443 and is compared with the old proxy host by host
  (status, redirects, headers, body).
- The cutover has a health gate and rolls back automatically.
- Hosts that still use the fallback are counted; `--finalize` refuses to run while any host depends on the old proxy.

## Afterwards

- Ingress, Traefik IngressRoute and Gateway API HTTPRoute objects keep working: the panel syncs them to gateway
  routes, and cert-manager Secrets become gateway certificates. Synced Ingresses get the edge address in their
  status (`kubectl get ingress`, Argo CD health).
- `--adopt` asks which existing namespaces become Nectar projects (`all`, names, or Enter for none);
  `--adopt-projects all|none|ns1,ns2` answers in advance, and `sh install.sh --adopt-projects ask` asks again
  later. The panel adopts them within a minute. From any computer: `nectar external`, `nectar adopt NS…|--all`.
- Existing namespaces are listed read-only on the **Projects** page ("Running on this cluster, not managed by Nectar"); adopt any of them as a project there, and its existing workloads stay visible on the project page. The panel only
  manages objects it creates.
- In an adopted project, **Import as app** (on each Deployment or single-replica StatefulSet) turns the workload
  into a Nectar app: image, ports, variables (also from Secrets/ConfigMaps), mounted config files, replicas,
  resources, health check and the same PVCs (no data copied). The original is scaled to 0 and its Services select
  the app's pods. Ingress hosts whose every path leads to the workload become the app's domains (HTTPS from
  Let's Encrypt; the panel's domains win over the Ingress, which stays in place unused); hosts shared with other
  workloads or using regex paths stay on the Ingress. Deleting the app scales the original back up, restores the
  Services and hands the hosts back to the Ingress. Not importable: several containers, init containers, host network, privileged containers, hostPath
  and other volume kinds.
- The built-in registry is not added unless you pass `--registry` (its mirror needs one k3s restart).
