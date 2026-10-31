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
- Existing namespaces show up read-only under **External**; adopt any of them as a project. The panel only
  manages objects it creates.
- The built-in registry is not added unless you pass `--registry` (its mirror needs one k3s restart).
