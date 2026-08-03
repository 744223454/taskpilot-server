#!/usr/bin/env bash

set -euo pipefail

api_log=$(mktemp)
worker_log=$(mktemp)
cookie_jar=$(mktemp)

cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		echo "API log:"
		cat "$api_log"
		echo "Worker log:"
		cat "$worker_log"
	fi
	if [ -n "${api_pid:-}" ]; then kill "$api_pid" 2>/dev/null || true; fi
	if [ -n "${worker_pid:-}" ]; then kill "$worker_pid" 2>/dev/null || true; fi
	rm -f "$api_log" "$worker_log" "$cookie_jar"
}
trap cleanup EXIT

./bin/taskpilot-worker -f etc/taskpilot-api.example.yaml >"$worker_log" 2>&1 &
worker_pid=$!
./bin/taskpilot-api -f etc/taskpilot-api.example.yaml >"$api_log" 2>&1 &
api_pid=$!

for _ in $(seq 1 30); do
	if curl --fail --silent http://127.0.0.1:8888/readyz >/dev/null; then
		break
	fi
	sleep 1
done
curl --fail --silent http://127.0.0.1:8888/readyz >/dev/null

email="ci-smoke-$(date +%s)-$$@example.com"
register_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
	--cookie-jar "$cookie_jar" \
	--header 'Content-Type: application/json' \
	--data "{\"email\":\"$email\",\"password\":\"taskpilot-ci-password\",\"nickname\":\"CI Smoke\"}" \
	http://127.0.0.1:8888/api/v1/auth/register)
test "$register_status" = "201"

me_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
	--cookie "$cookie_jar" \
	http://127.0.0.1:8888/api/v1/users/me)
test "$me_status" = "200"

missing_csrf_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
	--cookie "$cookie_jar" \
	--header 'Content-Type: application/json' \
	--data '{"title":"CI Smoke","text":"This request must be rejected without CSRF."}' \
	http://127.0.0.1:8888/api/v1/documents/text)
test "$missing_csrf_status" = "403"

csrf_token=$(awk '$6 == "csrf_token" { print $7 }' "$cookie_jar" | tail -n 1)
test -n "$csrf_token"

profile_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
	--cookie "$cookie_jar" \
	--cookie-jar "$cookie_jar" \
	--header 'Content-Type: application/json' \
	--header "X-CSRF-Token: $csrf_token" \
	--request PUT \
	--data '{"nickname":"CI Smoke Updated","avatar_url":null}' \
	http://127.0.0.1:8888/api/v1/users/me)
test "$profile_status" = "200"

csrf_token=$(awk '$6 == "csrf_token" { print $7 }' "$cookie_jar" | tail -n 1)
document_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
	--cookie "$cookie_jar" \
	--header 'Content-Type: application/json' \
	--header "X-CSRF-Token: $csrf_token" \
	--data '{"title":"CI Smoke","text":"This text document verifies the authenticated process-level smoke path."}' \
	http://127.0.0.1:8888/api/v1/documents/text)
test "$document_status" = "201"
