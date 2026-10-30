#!/bin/sh
# End-to-end smoke test against a running install (CI job "e2e", or a test server).
#
#   P=http://127.0.0.1:30480 GW=http://127.0.0.1 scripts/e2e.sh
#
# It sets up the owner (or logs in when the panel is already set up), deploys an image app with a secret
# environment variable, routes a domain through the gateway, deploys a static site from an uploaded zip,
# rotates the master key and checks that the secret survives a redeploy.
# Needs curl, python3, zip and kubectl access (KC, default "sudo k3s kubectl").
set -eu

P="${P:-http://127.0.0.1:30480}"
GW="${GW:-http://127.0.0.1}"
KC="${KC:-sudo k3s kubectl}"
EMAIL="${E2E_EMAIL:-owner@example.com}"
PASSWORD="${E2E_PASSWORD:-correct-horse-battery-9}"
IMAGE="${E2E_IMAGE:-nginxinc/nginx-unprivileged:1.29-alpine}"
PORT="${E2E_PORT:-8080}"
MATCH="${E2E_MATCH:-welcome to nginx}" # expected in the image app's answer
ARTIFACT="${E2E_ARTIFACT:-1}" # 0 skips the static-site part (needs the nginx-unprivileged image)
SLUG="${E2E_PROJECT:-e2e}"

WORK="$(mktemp -d)"
JAR="$WORK/cookies"
trap 'rm -rf "$WORK"' EXIT

step() { printf '\n== %s\n' "$*"; }
dump() {
	$KC get pods -A -o wide || true
	$KC -n nectar-system logs deploy/nectar-apps --tail=80 || true
	$KC -n nectar-system logs deploy/nectar-gateway --tail=40 || true
	$KC -n "prj-$SLUG" get events --sort-by=.lastTimestamp 2>/dev/null | tail -30 || true
}
fail() {
	printf 'FAIL: %s\n' "$*" >&2
	dump
	exit 1
}

# req METHOD PATH [JSON] → body in $WORK/body, status in $CODE
req() {
	if [ $# -ge 3 ]; then
		CODE="$(curl -sS -b "$JAR" -c "$JAR" -X "$1" -H 'Content-Type: application/json' -d "$3" -o "$WORK/body" -w '%{http_code}' "$P$2")"
	else
		CODE="$(curl -sS -b "$JAR" -c "$JAR" -X "$1" -o "$WORK/body" -w '%{http_code}' "$P$2")"
	fi
}
# expect CODE… : the last request answered one of these
expect() {
	for c in "$@"; do
		[ "$CODE" = "$c" ] && return 0
	done
	fail "$LAST → HTTP $CODE (want $*): $(head -c 400 "$WORK/body")"
}
# field EXPR: a Python expression over the JSON body d
field() { python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print($1)" "$WORK/body"; }
call() {
	LAST="$1 $2"
	req "$@"
}

# wait_for SECONDS DESCRIPTION COMMAND…
wait_for() {
	limit="$1" what="$2"
	shift 2
	i=0
	while ! "$@" >/dev/null 2>&1; do
		i=$((i + 2))
		[ "$i" -ge "$limit" ] && fail "timed out after ${limit}s: $what"
		sleep 2
	done
}

app_running() {
	call GET "/api/apps/$1"
	[ "$CODE" = 200 ] && [ "$(field "d['status']")" = running ]
}
routed() { curl -fsS -H "Host: $1" "$GW/" | grep -qi "$2"; }

step "panel answers"
wait_for 180 "panel at $P" curl -fsS "$P/api/setup"

step "owner"
call GET /api/setup
expect 200
if [ "$(field "d.get('needs_setup', d.get('required', False))")" = True ]; then
	token="$($KC -n nectar-system get secret nectar-secrets -o jsonpath='{.data.setup-token}' | base64 -d)"
	call POST /api/setup "{\"token\":\"$token\",\"email\":\"$EMAIL\",\"name\":\"Owner\",\"password\":\"$PASSWORD\"}"
	expect 200 201
else
	call POST /api/auth/login "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
	expect 200
fi
call GET /api/me
expect 200

step "project"
call POST /api/projects "{\"slug\":\"$SLUG\",\"name\":\"E2E\"}"
expect 201
pid="$(field "d['id']")"

step "image app with a secret variable"
call POST "/api/projects/$pid/apps" "{\"slug\":\"web\",\"name\":\"Web\",\"runtime\":\"image\",\"config\":{\"image\":\"$IMAGE\",\"port\":$PORT}}"
expect 201
web="$(field "d['id']")"
call PUT "/api/apps/$web/env" '[{"key":"E2E_SECRET","value":"sealed-value-1","secret":true}]'
expect 200 204
call POST "/api/apps/$web/deploy"
expect 200 201 202
wait_for 300 "app web running" app_running "$web"

step "domain through the gateway"
call POST "/api/apps/$web/domains" '{"host":"web.e2e.test"}'
expect 201
wait_for 120 "web.e2e.test served by the gateway" routed web.e2e.test "$MATCH"

if [ "$ARTIFACT" = 1 ]; then
	step "static site from an uploaded zip"
	mkdir -p "$WORK/site"
	echo '<h1>nectar-e2e-static</h1>' >"$WORK/site/index.html"
	(cd "$WORK/site" && zip -q ../site.zip index.html)
	call POST "/api/projects/$pid/apps" '{"slug":"site","name":"Site","runtime":"static"}'
	expect 201
	site="$(field "d['id']")"
	LAST="upload site.zip"
	CODE="$(curl -sS -b "$JAR" -X POST -H 'Content-Type: application/zip' --data-binary "@$WORK/site.zip" \
		-o "$WORK/body" -w '%{http_code}' "$P/api/apps/$site/artifacts?filename=site.zip")"
	expect 201
	call POST "/api/apps/$site/deploy"
	expect 200 201 202
	wait_for 300 "app site running" app_running "$site"
	call POST "/api/apps/$site/domains" '{"host":"site.e2e.test"}'
	expect 201
	wait_for 120 "site.e2e.test served" routed site.e2e.test nectar-e2e-static
fi

env_value() {
	$KC -n "prj-$SLUG" get secret web-env -o jsonpath='{.data.E2E_SECRET}' | base64 -d
}
step "secret variable reached the cluster"
[ "$(env_value)" = sealed-value-1 ] || fail "web-env E2E_SECRET = '$(env_value)'"

step "master key rotation"
$KC -n nectar-system exec deploy/nectar-apps -- nectar-apps admin rotate-master-key >"$WORK/rotate.txt" 2>&1 ||
	fail "rotate-master-key: $(cat "$WORK/rotate.txt")"
grep -q "NEW MASTER KEY" "$WORK/rotate.txt" || fail "rotate-master-key printed: $(cat "$WORK/rotate.txt")"
sleep 5
$KC -n nectar-system rollout status deploy/nectar-apps --timeout=180s
wait_for 120 "panel back after the rotation" curl -fsS "$P/api/setup"
$KC -n nectar-system logs deploy/nectar-apps | grep -q "master key rotation finished" || fail "no rotation in the panel log"
next="$($KC -n nectar-system get secret nectar-secrets -o jsonpath='{.data.master-key-next}')"
[ -z "$next" ] || fail "master-key-next is still in the Secret"
call GET /api/me
expect 200 # sessions survive
call PUT "/api/apps/$web/env" '[{"key":"E2E_SECRET","value":"sealed-value-2","secret":true}]'
expect 200 204
call POST "/api/apps/$web/deploy"
expect 200 201 202
secret_is() { [ "$(env_value)" = "$1" ]; }
wait_for 120 "E2E_SECRET re-sealed and redeployed after the rotation" secret_is sealed-value-2
wait_for 300 "app web running after the rotation" app_running "$web"

step "cleanup"
call DELETE "/api/projects/$pid"
expect 200 202 204

printf '\nE2E OK\n'
