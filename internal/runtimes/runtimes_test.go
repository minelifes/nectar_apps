package runtimes

import (
	"strings"
	"testing"
)

func TestBuiltinCatalog(t *testing.T) {
	c, err := LoadBuiltin()
	if err != nil {
		t.Fatal(err)
	}
	java, ok := c.Get("java")
	if !ok || java.Artifact.Kind != ArtifactFile || java.Port != 8080 {
		t.Fatalf("java runtime: %+v", java)
	}
	if v, _ := java.Version(""); v.ID != "21" {
		t.Errorf("default java version = %s, want 21", v.ID)
	}
	if v, ok := java.Version("17"); !ok || !strings.HasPrefix(v.Image, "eclipse-temurin:17") {
		t.Errorf("java 17 = %+v", v)
	}
	if _, ok := java.Version("6"); ok {
		t.Error("java 6 should not exist")
	}
	static, _ := c.Get("static")
	if static.Supports(KindWorker) || len(static.ConfigFiles) != 1 || static.OptionDefaults()["spa_fallback"] != true {
		t.Errorf("static runtime: %+v", static)
	}
	img, _ := c.Get("image")
	if img.Artifact.Kind != ArtifactNone {
		t.Error("image runtime must not take an artifact")
	}
}

func TestValidation(t *testing.T) {
	bad := []string{
		`runtimes: []`,
		`runtimes: [{id: a, kinds: [web], artifact: {kind: file, path: rel}, versions: [{id: x, image: y}], port: 1, health: {type: tcp}}]`,
		`runtimes: [{id: a, kinds: [blah], artifact: {kind: none}, port: 1, health: {type: tcp}}]`,
		`runtimes: [{id: a, kinds: [web], artifact: {kind: none}, port: 0, health: {type: tcp}}]`,
		`runtimes: [{id: a, kinds: [web], artifact: {kind: file, path: /a}, versions: [], port: 1, health: {type: tcp}}]`,
	}
	for _, b := range bad {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("accepted invalid catalog: %s", b)
		}
	}
}
