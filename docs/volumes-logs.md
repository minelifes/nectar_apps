# Volumes and logs

## Volumes

- Add a **volume** (persistent disk) to an app with a mount path and size.
- **Files** (app → Settings → Volumes → Files): browse a volume, upload and download files (up to 64 MB),
  edit text files, create folders, rename and delete — also while the app is stopped and for images without a
  shell. A small helper pod mounts the volume while the dialog is open (an hour at most). Needs the
  `volume.manage` permission; every change is audited.
- **Upload .zip** (in Files): the archive is uploaded in 8 MB pieces and unpacked into the open folder —
  files with the same names are replaced, nothing else is deleted, new files get the folder's owner. Up to
  10 GB by default (`NECTAR_MAX_ZIP_MB`); the zip waits on the panel's disk while it is unpacked, so the
  panel checks its own free space and the volume's first. Paths that leave the folder refuse the archive;
  symlinks are skipped, `__MACOSX` and `.DS_Store` left out.
- **Sharing a volume between apps** (Settings → Volumes → *Use a volume of another app*): an app mounts a
  volume of another app of the same project, optionally read-only, and both see the same files. The volume
  stays with its app — size, storage class and backups are set there, and it can't be removed or deleted
  there (nor that app deleted) while another app mounts it. A ReadWriteOnce volume lives on one node, so the
  second app prefers that node; for apps on different nodes make it a **shared** (ReadWriteMany) volume.
  PR previews get an empty volume of their own unless *preview databases* is on. Compose imports share named
  volumes the same way; in `nectar.yaml` it is `from: app/volume` (+ `read_only: true`).
- **Usage:** each volume shows how much it holds: the app's Settings → Volumes, the database page and
  Server → Volumes. The **Volume almost full** alert (90 % by default) warns early. On the default
  local-path storage a volume is a folder on the server's disk. Kubernetes cannot tell a folder's size, so the
  panel measures these folders itself every 10 minutes, with a short read-only `du` pod per node. The numbers
  appear a few minutes after the panel starts. Sizes are not enforced on local-path: a volume can grow past
  its size until the node's disk is full, which is why the alert matters.
- **Storage classes:** local-path (one node) by default; `install.sh --longhorn` adds Longhorn (replicated
  across nodes), `install.sh --nfs 10.0.0.5:/srv/nectar` an `nfs` class whose volumes several replicas on
  several nodes can share (**shared** volumes, ReadWriteMany). The class is chosen per volume and fixed
  afterwards; `--join` with the same flag installs the storage client on a new node.

## Logs

The Logs tab has **Live** (the stream of every pod) and **History**: the panel follows the pods itself and
keeps their logs on its disk (7 days, up to 512 MB per app by default — Server → Disk → Retention). Search by
text or regular expression over the last hour up to everything kept, also across crashes, restarts and deploys.
History is stored per hour and compressed once the hour is over; deleting an app deletes its logs.

## Metrics and events

App pages show CPU, memory and restarts per pod; Server shows nodes, disk and Kubernetes events. The
**gateway** records requests per domain (status, latency, bytes) for the traffic charts and the access log.
