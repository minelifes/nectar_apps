package deploy

import (
	"context"
	"net/url"
)

// pvcStat is the usage of one PersistentVolumeClaim from a kubelet summary.
type pvcStat struct {
	UsedB, AvailableB int64
}

// kubeletSummary is the part of /stats/summary the volume usage needs.
type kubeletSummary struct {
	Node struct {
		Fs      fsStats `json:"fs"`
		Runtime struct {
			ImageFs fsStats `json:"imageFs"`
		} `json:"runtime"`
	} `json:"node"`
	Pods []struct {
		Volume []struct {
			fsStats
			PVCRef *struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"pvcRef"`
		} `json:"volume"`
	} `json:"pods"`
}

type fsStats struct {
	CapacityBytes  *int64 `json:"capacityBytes"`
	UsedBytes      *int64 `json:"usedBytes"`
	AvailableBytes *int64 `json:"availableBytes"`
}

// pvcStats returns the usage of the PVCs mounted on a node, keyed "namespace/name". A volume that is just a
// folder on the node's disk (k3s local-path) is reported by the kubelet with the statistics of the whole
// filesystem: its "used" bytes are the disk's (OS, images, every other volume) and would raise false
// "volume full" alerts. Those get UsedB -1 (unknown) and keep the free space of the disk.
func pvcStats(sum kubeletSummary) map[string]pvcStat {
	whole := map[int64]bool{}
	for _, fs := range []fsStats{sum.Node.Fs, sum.Node.Runtime.ImageFs} {
		if fs.CapacityBytes != nil && *fs.CapacityBytes > 0 {
			whole[*fs.CapacityBytes] = true
		}
	}
	out := map[string]pvcStat{}
	for _, p := range sum.Pods {
		for _, v := range p.Volume {
			if v.PVCRef == nil || v.UsedBytes == nil {
				continue
			}
			st := pvcStat{UsedB: *v.UsedBytes, AvailableB: -1}
			if v.AvailableBytes != nil {
				st.AvailableB = *v.AvailableBytes
			}
			if v.CapacityBytes != nil && whole[*v.CapacityBytes] {
				st.UsedB = -1 // the node's filesystem, not this volume
			}
			key := v.PVCRef.Namespace + "/" + v.PVCRef.Name
			if prev, ok := out[key]; !ok || st.UsedB > prev.UsedB {
				out[key] = st
			}
		}
	}
	return out
}

// nodePVCStats reads one node's kubelet summary (best effort: nil on errors).
func nodePVCStats(ctx context.Context, rg rawGetter, node string) map[string]pvcStat {
	var sum kubeletSummary
	if rg.Get(ctx, "/api/v1/nodes/"+url.PathEscape(node)+"/proxy/stats/summary", &sum) != nil {
		return nil
	}
	return pvcStats(sum)
}
