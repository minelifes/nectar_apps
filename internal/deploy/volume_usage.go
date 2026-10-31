package deploy

import (
	"context"
	"sort"
	"strconv"

	"nectar_apps/internal/kube"
)

// VolumeUse is one Nectar volume (an app volume or a database's data) with its usage.
type VolumeUse struct {
	Namespace    string `json:"namespace"`
	Claim        string `json:"claim"`
	Project      string `json:"project"`          // project slug
	App          string `json:"app,omitempty"`    // app slug (app volumes)
	AppID        int64  `json:"app_id,omitempty"` // app id (app volumes)
	Volume       string `json:"volume,omitempty"` // volume name in the app settings
	Addon        string `json:"addon,omitempty"`  // add-on slug (database data)
	RequestedB   int64  `json:"requested_bytes"`  // the size in the settings
	UsedB        int64  `json:"used_bytes"`       // -1 = unknown (no pod mounts it right now)
	AvailableB   int64  `json:"available_bytes"`  // free space where it lives (-1 = unknown)
	StorageClass string `json:"storage_class,omitempty"`
	Phase        string `json:"phase"`
	Node         string `json:"node,omitempty"` // node of the pod that mounts it
}

// AllVolumes lists every Nectar volume in the cluster with its usage from the kubelet summaries. Usage is
// only known while a pod mounts the volume. On local-path storage the size is not enforced: a volume can
// grow past it until the node's disk is full (AvailableB is the node's free space then).
func (d *Deployer) AllVolumes(ctx context.Context) ([]VolumeUse, error) {
	pvcs, err := d.kube.List(ctx, "PersistentVolumeClaim", "", LabelManagedBy+"="+ManagedBy)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*VolumeUse{}
	var out []*VolumeUse
	for _, pvc := range pvcs {
		ns, name := kube.Str(pvc, "metadata", "namespace"), kube.Str(pvc, "metadata", "name")
		v := &VolumeUse{
			Namespace: ns, Claim: name, UsedB: -1, AvailableB: -1,
			Project: kube.Str(pvc, "metadata", "labels", LabelProject),
			App:     kube.Str(pvc, "metadata", "labels", LabelApp),
			Volume:  kube.Str(pvc, "metadata", "labels", LabelVolume),
			Addon:   kube.Str(pvc, "metadata", "labels", LabelAddon),
			Phase:   kube.Str(pvc, "status", "phase"), StorageClass: kube.Str(pvc, "spec", "storageClassName"),
		}
		if id, err := strconv.ParseInt(kube.Str(pvc, "metadata", "labels", LabelAppID), 10, 64); err == nil {
			v.AppID = id
		}
		if q, err := kube.ParseQuantity(kube.Str(pvc, "spec", "resources", "requests", "storage")); err == nil {
			v.RequestedB = int64(q)
		}
		byKey[ns+"/"+name] = v
		out = append(out, v)
	}
	if rg, ok := d.kube.(rawGetter); ok && len(out) > 0 {
		nodes, err := d.kube.List(ctx, "Node", "", "")
		if err == nil {
			for _, n := range nodes {
				node := kube.Str(n, "metadata", "name")
				for key, st := range nodePVCStats(ctx, rg, node) {
					v, ok := byKey[key]
					if !ok || (st.UsedB < v.UsedB && v.Node != "") {
						continue
					}
					v.UsedB, v.AvailableB, v.Node = st.UsedB, st.AvailableB, node
				}
			}
		}
	}
	// node-local folders (k3s local-path): the kubelet cannot tell their size, the volume meter measures them
	for _, v := range out {
		if v.UsedB < 0 {
			if b, ok := d.measuredUsage(v.Namespace + "/" + v.Claim); ok {
				v.UsedB = b
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Claim < out[j].Claim
	})
	res := make([]VolumeUse, len(out))
	for i, v := range out {
		res[i] = *v
	}
	return res, nil
}
