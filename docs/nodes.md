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
