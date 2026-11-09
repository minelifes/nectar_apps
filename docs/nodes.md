# Several servers and edge nodes

## Add a node

**Server → Add node** shows a one-line command for another Linux machine: it installs k3s as an agent, joins
the cluster and sets up the registry mirror (and Longhorn/NFS clients if you use them). Apps then spread over
the nodes.

Each node card can:

- stop new pods (**cordon**) and allow them again;
- move its pods away (**drain**, respecting disruption budgets);
- remove a drained worker from the cluster.

Draining the edge node, or the last node that could take the pods, asks again first: it would take the gateway
or the panel offline.

## Labels and app placement

Give nodes labels (node card → **Labels…**, or `nectar node label NODE disk=ssd gpu-`), e.g. `disk=ssd`,
`gpu=nvidia`, `zone=office`. Then in an app's **Settings → Placement** pick the labels it needs: its pods
(and its release and task jobs) run only on nodes that have all of them. Nothing picked: any node.

- Labels of Kubernetes, k3s and Nectar (`kubernetes.io/…`, `k3s.io/…`, `nectar.dev/…`, …) are not shown or
  changed.
- If no ready node has the labels, new pods wait (Pending) — the settings page warns, and removing a label an
  app needs tells you which apps are affected. Running pods are not moved when labels change; redeploy or
  restart the app.
- An app with a ReadWriteOnce volume stays on the node that holds the volume: pick labels that node has.

## Several edge nodes

By default nectar-gateway runs on one **edge node** (label `nectar.dev/edge=true`, set by `install.sh`) — the
node your DNS points to. To serve traffic from more nodes:

1. Open ports 80 and 443 on the other machine.
2. On its node card choose **Serve traffic too (edge node)**.

The panel labels the node `nectar.dev/edge-replica=true` and runs a **gateway replica** there (DaemonSet
`nectar-gateway-edge`, same image as the main gateway). A replica:

- copies routes, certificates, IP blocks and settings from the main gateway every 5 seconds;
- answers Let's Encrypt HTTP-01 challenges through the main gateway, so certificates are issued whichever node
  Let's Encrypt reaches;
- is read-only and reports "not ready" until its first copy;
- keeps its own rate-limit blocks.

The node card shows whether the replica is in sync. DNS records the panel manages then list **every edge
node** (round-robin DNS). If an edge node goes down for long, choose **Stop serving traffic** so the panel takes
it out of DNS. Needs nectar-gateway with the `replicas` feature (the latest release).
