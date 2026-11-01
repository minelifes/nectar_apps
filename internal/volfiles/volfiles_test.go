package volfiles

import (
	"testing"

	"nectar_apps/internal/kube"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"":            "/data",
		"/":           "/data",
		"logs":        "/data/logs",
		"/logs/a.txt": "/data/logs/a.txt",
		"a/../b":      "/data/b",
		"../../etc":   "/data/etc", // Clean of "/../../etc" stays inside
		"a\x00b":      "",
		"a\nb":        "",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if n := PodName("very-long-app-name-for-a-claim-that-goes-on-and-on-forever-and-ever"); len(n) > 63 {
		t.Fatalf("pod name too long: %d", len(n))
	}
}

func TestParseList(t *testing.T) {
	out := "regular file|12|1700000000|644|1000:1000|./b.txt\n" +
		"directory|4096|1700000001|755|0:0|./logs\n" +
		"symbolic link|7|1700000002|777|0:0|./current\n" +
		"regular empty file|0|1700000003|600|1000:1000|./.env\n" +
		"garbage line\n"
	list := parseList(out)
	if len(list) != 4 || list[0].Name != "logs" || list[0].Type != "dir" || list[1].Name != "b.txt" || list[1].Size != 12 ||
		list[2].Type != "link" || list[3].Type != "file" || list[3].Owner != "1000:1000" || list[1].Modified.Unix() != 1700000000 {
		t.Fatalf("parsed: %+v", list)
	}
}

// Project namespaces enforce PodSecurity "baseline": a container may only add capabilities of the default
// set, or the API server rejects the pod ("non-default capabilities"). DAC_READ_SEARCH was one of those.
func TestHelperPodFitsBaseline(t *testing.T) {
	baseline := map[string]bool{"AUDIT_WRITE": true, "CHOWN": true, "DAC_OVERRIDE": true, "FOWNER": true, "FSETID": true, "KILL": true,
		"MKNOD": true, "NET_BIND_SERVICE": true, "SETFCAP": true, "SETGID": true, "SETPCAP": true, "SETUID": true, "SYS_CHROOT": true}
	pod := helperPod("prj-a", "nectar-files-x", "x", "busybox:1.37")
	for _, c := range kube.Slice(pod, "spec", "containers") {
		adds, _ := kube.Field(c, "securityContext", "capabilities", "add").([]any)
		for _, a := range adds {
			if !baseline[a.(string)] {
				t.Errorf("container %s adds %v, which baseline rejects", kube.Str(c, "name"), a)
			}
		}
	}
}
