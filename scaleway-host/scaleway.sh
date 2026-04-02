#!/bin/bash

set -eEuo pipefail

CM_QUIET=${CM_QUIET:-false}
SCW_IS_SOURCED=false
SCW_SCRIPT_DIR=$(cd -- "$( dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
SCW_WG_THIS_ENDPOINT="89.167.110.237"
SCW_WG_THIS_PORT="51871"
SCW_WG_THIS_INTERNAL="10.13.13.1/32"
SCW_REMOTE_USERNAME="cogshelium00"
SCW_CLOUDINITS_PATH="$SCW_SCRIPT_DIR/cloudinits"
SCW_TERRAFORM_PATH="$SCW_SCRIPT_DIR/main.tf"

if [[ "$0" != "${BASH_SOURCE[0]}" ]]; then
	SCW_IS_SOURCED=true
	CM_QUIET=true
fi

source $SCW_SCRIPT_DIR/common/log.sh
source $SCW_SCRIPT_DIR/wireguard/wireguard.sh
source $SCW_SCRIPT_DIR/.venv/bin/activate

if ! $CM_QUIET; then
	log_info "Script base path detected: $SCW_SCRIPT_DIR"
fi

help() {
	help_page=$(
		cat <<'EOF'
scaleway.sh -- scaleway helper tool
* generate_cloudinit
* run
EOF
	)

	log_info "$help_page"
}

scw_generate_cloudinit() {
	mkdir -p "$SCW_CLOUDINITS_PATH"
	log_wait "Generating cloudinit..."

	if (( "$#" < 8 )) || (( "$#" > 9 )); then
		log_err "Usage: <private_key> <address> \
<public_key> <endpoint_ip> <endpoint_ip_port> <allowedips> <psk> <ssh_pubkey>"
		exit 1
	fi

	local private_key="$1"
	local address="$2"
	local public_key="$3"
	local endpoint_ip="$4"
	local endpoint_ip_port="$5"
	local allowedips="$6"
	local preshared_key="$7"
	local ssh_public_key="$8"
	local dest_path="${9:-$SCW_SCRIPT_DIR/user-data.yml}"

	jinja2 \
		--strict \
		"$SCW_SCRIPT_DIR/user-data.j2" \
		- \
		--format json \
		<<EOF >"$dest_path" 2>/dev/null
{
	"private_key": "$private_key",
	"address": "$address",
	"public_key": "$public_key",
	"endpoint_ip": "$endpoint_ip",
	"endpoint_ip_port": "$endpoint_ip_port",
	"allowedips": "$allowedips",
	"preshared_key": "$preshared_key",
	"ssh_public_key": "$ssh_public_key"
}
EOF
	log_ok "Cloudinit ready"
}

__scw_wait_for_start() {
	local nocidr="$1"

	until scw_ssh "$nocidr" "uname -a" &>/dev/null; do
		log_wait "Waiting for SSH to work"
		sleep 3
	done

	log_ok "SSH started @ ${nocidr}"
}

scw_ssh() {
	local nocidr="$1"
	shift

	ssh ${SCW_REMOTE_USERNAME}@${nocidr} \
		-o StrictHostKeyChecking=no \
		-o UserKnownHostsFile=/dev/null \
		"$@"
}

scw_upload() {
	local nocidr="$1"
	local local_path="$2"
	local remote_path="$3"
	local options="${4:-""}"

	scp \
		-o StrictHostKeyChecking=no \
		-o UserKnownHostsFile=/dev/null \
		$options \
		"$local_path" \
		${SCW_REMOTE_USERNAME}@${nocidr}:${remote_path}
}

scw_download() {
	local nocidr="$1"
	local remote_path="$2"
	local local_path="$3"
	local options="${4:-""}"

	scp \
		-o StrictHostKeyChecking=no \
		-o UserKnownHostsFile=/dev/null \
		$options \
		${SCW_REMOTE_USERNAME}@${nocidr}:${remote_path} \
		"$local_path"	
}

scw_generate_terraform() {
	local instances_json num_workers
	
	if [[ "$#" != "1" ]]; then
		log_err "Usage: <n>"
		exit 1
	fi

	num_workers="$1"
	instances_json="{\"instances\": ["

	for i in $(seq 1 "$num_workers"); do
		instances_json="${instances_json}{}"
		if [[ "$i" != "$num_workers" ]]; then
			instances_json="${instances_json},"
		fi
	done

	instances_json="${instances_json}]}"

	jinja2 \
		--strict \
		"$SCW_SCRIPT_DIR/main.j2" \
		<(echo "$instances_json") \
		--format json \
		>"$SCW_TERRAFORM_PATH" 2>/dev/null
}

scw_start() {
	local num_workers workers_nocidr_ips workers_wg_pubkeys nocidr json

	if (( $# > 1 )); then
		log_err "Usage: [n] [json]"
		exit 1
	fi

	num_workers="${1:-1}"
	json="${2:-no}"

	workers_nocidr_ips=()
	workers_wg_pubkeys=()

	log_wait "Generating terraform"
	scw_generate_terraform "$num_workers"

	for i in $(seq 1 "$num_workers"); do
		log_wait "Generating a new peer [$i]"
		eval "$(__wg_hotnew)"

		log_wait "Generating cloudinit for a new peer [$i]"
		scw_generate_cloudinit \
			"$wg_hotnew_private_key" \
			"$wg_hotnew_ip_addr" \
			"$(wg show wg0 public-key)" \
			"$SCW_WG_THIS_ENDPOINT" \
			"$SCW_WG_THIS_PORT" \
			"$SCW_WG_THIS_INTERNAL" \
			"$wg_hotnew_psk" \
			"$(cat /root/.ssh/id_ed25519.pub)" \
			"${SCW_CLOUDINITS_PATH}/${i}.yml" &>/dev/null

		log_wait "Adding a new peer [$i]"
		__wg_hotadd \
			"$wg_hotnew_public_key" \
			"$wg_hotnew_ip_addr" \
			"$wg_hotnew_psk" &>/dev/null
		
		nocidr=$(echo "$wg_hotnew_ip_addr" | awk -F'/' '{print $1}')
		log_info "Nocidr [$i] ip is '$nocidr'"
		workers_nocidr_ips+=("$nocidr")
		workers_wg_pubkeys+=("$wg_hotnew_public_key")
	done

	log_wait "Tofu plan"
	(cd "$SCW_SCRIPT_DIR" && tofu plan &>/dev/null)

	log_wait "Starting worker instances"
	(cd "$SCW_SCRIPT_DIR" && tofu apply -auto-approve &>/dev/null)

	for i in "${!workers_nocidr_ips[@]}"; do
		__scw_wait_for_start "${workers_nocidr_ips[i]}"
		log_ok "Instance [$(( i+1 ))] started"
	done

	if [[ "$json" == "yes" ]]; then
		printf "{\"workers\": ["

		for i in "${!workers_wg_pubkeys[@]}"; do
			printf "{"
			printf "pubkey: \"${workers_wg_pubkeys[i]}\","
			printf "ipaddr: \"${workers_nocidr_ips[i]}\""
			printf "}"

			if (( i != num_workers-1 )); then
				printf ","
			fi
		done

		printf "]}\n"
	else
		echo "local scw_wg_public_keys=(${workers_wg_pubkeys[@]@Q})"
		echo "local scw_nocidrs=(${workers_nocidr_ips[@]@Q})"
	fi
}

scw_stop() {
	local hot_peer_pubkeys=("$@")

	log_wait "Stopping instances"
	(cd "$SCW_SCRIPT_DIR" && tofu destroy -auto-approve)

	log_wait "Removing the hot peers"

	for hot_peer_pubkey in "${hot_peer_pubkeys[@]}"; do
		wg_hotdel "$hot_peer_pubkey"
	done

	log_ok "Instances stopped"
}

scw_run() {
	local num_workers="${1:-1}"

	eval "$(scw_start "$num_workers")"
	log_wait "Waiting 200 seconds"

	for i in {1..20}; do
		sleep 10
		log_wait "[${i}/20]"
	done

	scw_stop "${scw_wg_public_keys[@]}"
}

scw_cli() {
	numargs=$#

	if [[ "$numargs" == "0" ]]; then
		local command="help"
		local args=""
	else
		local command="$1"
		shift
		local args="$@"
	fi


	case $command in
			help|run|start|stop|ssh|scp|generate_terraform)
			local prefixed_command="scw_${command}"
			$prefixed_command $args
			;;
		*)
			echo "ERROR: Unknown command"
			;;
	esac
}

if ! $SCW_IS_SOURCED; then
	scw_cli $@
fi
