#!/usr/bin/env bash
# Provision the operator trust anchor used only for stale/unverifiable v7 cutover.
set -euo pipefail
umask 077

if (( EUID != 0 )); then
	printf '%s\n' 'error: run this provisioning script as root' >&2
	exit 1
fi

trust_dir=/etc/agent-team
private_key=/root/agent-team-cutover-ed25519.pem
if [[ ${AGENT_TEAM_TEST_MODE:-} == 1 ]]; then
	: "${DEST_DIR:?error: DEST_DIR is required in test mode}"
	: "${PRIVATE_KEY:?error: PRIVATE_KEY is required in test mode}"
	case "$DEST_DIR:$PRIVATE_KEY" in
		/tmp/*:/tmp/*) ;;
		*) printf '%s\n' 'error: test-mode paths must be absolute paths below /tmp' >&2; exit 1 ;;
	esac
	trust_dir=$DEST_DIR
	private_key=$PRIVATE_KEY
elif [[ -n ${DEST_DIR:-} || -n ${PRIVATE_KEY:-} ]]; then
	printf '%s\n' 'error: path overrides require AGENT_TEAM_TEST_MODE=1' >&2
	exit 1
fi

for command in openssl jq sha256sum base64 install tail stat tr mktemp sync ln rm; do
	command -v "$command" >/dev/null 2>&1 || {
		printf 'error: required command is unavailable: %s\n' "$command" >&2
		exit 1
	}
done

trust_store=$trust_dir/cutover-trust.json
private_temp=$private_key.tmp
trust_temp=$trust_store.tmp
for path in "$private_key" "$trust_store" "$private_temp" "$trust_temp"; do
	if [[ -e $path || -L $path ]]; then
		printf 'error: refusing to overwrite existing path: %s\n' "$path" >&2
		exit 1
	fi
done

work=$(mktemp -d)
cleanup() {
	rm -f -- "$private_temp" "$trust_temp"
	rm -rf -- "$work"
}
trap cleanup EXIT HUP INT TERM

generated_private=$work/operator-private.pem
public_der=$work/operator-public.der
public_raw=$work/operator-public.raw
trust_json=$work/cutover-trust.json

openssl genpkey -algorithm Ed25519 -out "$generated_private" >/dev/null 2>&1
openssl pkey -in "$generated_private" -check -noout >/dev/null 2>&1
openssl pkey -in "$generated_private" -pubout -outform DER -out "$public_der"
[[ $(stat -c %s "$public_der") == 44 ]] || {
	printf '%s\n' 'error: unexpected Ed25519 public DER size' >&2
	exit 1
}
tail -c 32 "$public_der" >"$public_raw"
[[ $(stat -c %s "$public_raw") == 32 ]] || {
	printf '%s\n' 'error: unexpected Ed25519 raw public key size' >&2
	exit 1
}
key_id=$(sha256sum "$public_raw" | { read -r digest _; printf '%s' "$digest"; })
[[ $key_id =~ ^[0-9a-f]{64}$ ]] || {
	printf '%s\n' 'error: invalid public key digest' >&2
	exit 1
}
public_key=$(base64 <"$public_raw" | tr -d '\r\n')
jq -cn --arg id "$key_id" --arg key "$public_key" \
	'{schema:1,keys:[{algorithm:"ed25519",keyId:$id,publicKey:$key}]}' >"$trust_json"
jq -e --arg id "$key_id" \
	'.schema == 1 and (.keys | length) == 1 and .keys[0].algorithm == "ed25519" and .keys[0].keyId == $id and ([.keys[].keyId] == ([.keys[].keyId] | sort | unique))' \
	"$trust_json" >/dev/null

install -d -o root -g root -m 0755 "$trust_dir"
install -o root -g root -m 0600 "$generated_private" "$private_temp"
install -o root -g root -m 0644 "$trust_json" "$trust_temp"
sync -f "$private_temp" "$trust_temp" "$trust_dir" 2>/dev/null || sync

# Hard links publish without overwriting a path created after the checks above.
ln "$private_temp" "$private_key"
ln "$trust_temp" "$trust_store" || {
	rm -f -- "$private_key"
	exit 1
}
rm -f -- "$private_temp" "$trust_temp"
sync -f "$private_key" "$trust_store" "$trust_dir" 2>/dev/null || sync

[[ $(stat -c '%u:%a' "$private_key") == 0:600 ]]
[[ $(stat -c '%u:%a' "$trust_store") == 0:644 ]]
[[ $(stat -c '%u:%a' "$trust_dir") == 0:755 ]]
openssl pkey -in "$private_key" -check -noout >/dev/null 2>&1
jq -e --arg id "$key_id" '.schema == 1 and .keys[0].keyId == $id' "$trust_store" >/dev/null

trap - EXIT HUP INT TERM
rm -rf -- "$work"
printf 'key id: %s\nprivate key: %s\ntrust store: %s\n' "$key_id" "$private_key" "$trust_store"
