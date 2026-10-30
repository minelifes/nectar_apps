package housekeep

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"nectar_apps/internal/kube"
)

// PrepullName is the DaemonSet that keeps images on every node.
const PrepullName = "nectar-prepull"

// maxPrepullImages limits the extra images in the settings.
const maxPrepullImages = 20

// Prepull keeps the init image (busybox) and the runtime images in use on every node, so a new node or a
// rescheduled pod starts without pulling and the image cleanup ("crictl rmi --prune") never removes them.
//
// Each image is a container of the DaemonSet that only sleeps: an init container copies busybox into a shared
// volume and every other container runs "/nectar/busybox sleep", so images without a shell (distroless) work too.
// The containers run as nobody without capabilities and request 1m CPU / 4 MiB each.
//
// With pre-pull switched off the DaemonSet is removed.
func (k *Keeper) SyncPrepull(ctx context.Context) error {
	if k.Kube == nil {
		return nil
	}
	set := Load(ctx, k.Store)
	if !set.Prepull {
		err := k.Kube.Delete(ctx, "DaemonSet", k.Namespace, PrepullName)
		if err != nil && !kube.IsNotFound(err) {
			return err
		}
		return nil
	}
	if k.InitImage == "" {
		return errors.New("prepull: no init image")
	}
	var images []string
	if k.Images != nil {
		images = append(images, k.Images(ctx)...)
	}
	images = append(images, set.PrepullImages...)
	_, err := k.Kube.Apply(ctx, prepullDaemonSet(k.Namespace, k.InitImage, images))
	return err
}

// PrepullImages returns the images the DaemonSet keeps (init image first, then sorted, without duplicates).
func prepullImages(initImage string, images []string) []string {
	seen := map[string]bool{initImage: true}
	var rest []string
	for _, img := range images {
		img = strings.TrimSpace(img)
		if img == "" || seen[img] {
			continue
		}
		seen[img] = true
		rest = append(rest, img)
	}
	slices.Sort(rest)
	return append([]string{initImage}, rest...)
}

func prepullDaemonSet(ns, initImage string, images []string) kube.Object {
	labels := map[string]any{"app.kubernetes.io/name": PrepullName, "app.kubernetes.io/managed-by": "nectar-apps"}
	sc := map[string]any{
		"runAsUser": 65534, "runAsGroup": 65534, "runAsNonRoot": true,
		"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true,
		"capabilities":   map[string]any{"drop": []any{"ALL"}},
		"seccompProfile": map[string]any{"type": "RuntimeDefault"},
	}
	res := map[string]any{
		"requests": map[string]any{"cpu": "1m", "memory": "4Mi"},
		"limits":   map[string]any{"memory": "16Mi"},
	}
	mount := []any{map[string]any{"name": "bin", "mountPath": "/nectar", "readOnly": true}}
	list := prepullImages(initImage, images)
	containers := make([]any, 0, len(list))
	for i, img := range list {
		containers = append(containers, map[string]any{
			"name":            fmt.Sprintf("image-%d", i),
			"image":           img,
			"imagePullPolicy": "IfNotPresent",
			"command":         []any{"/nectar/busybox", "sleep", "2147483647"},
			"securityContext": sc,
			"resources":       res,
			"volumeMounts":    mount,
		})
	}
	return kube.Object{
		"apiVersion": "apps/v1",
		"kind":       "DaemonSet",
		"metadata": map[string]any{
			"name": PrepullName, "namespace": ns, "labels": labels,
			"annotations": map[string]any{"nectar.dev/images": strings.Join(list, ",")},
		},
		"spec": map[string]any{
			"selector":       map[string]any{"matchLabels": map[string]any{"app.kubernetes.io/name": PrepullName}},
			"updateStrategy": map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"maxUnavailable": "100%"}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"automountServiceAccountToken":  false,
					"enableServiceLinks":            false,
					"terminationGracePeriodSeconds": 1,
					"tolerations":                   []any{map[string]any{"operator": "Exists"}},
					"initContainers": []any{map[string]any{
						"name":            "busybox",
						"image":           initImage,
						"imagePullPolicy": "IfNotPresent",
						"command":         []any{"cp", "/bin/busybox", "/nectar/busybox"},
						"securityContext": sc,
						"resources":       res,
						"volumeMounts":    []any{map[string]any{"name": "bin", "mountPath": "/nectar"}},
					}},
					"containers": containers,
					"volumes":    []any{map[string]any{"name": "bin", "emptyDir": map[string]any{"medium": "Memory", "sizeLimit": "8Mi"}}},
				},
			},
		},
	}
}
