#!/bin/bash

set -Eeuo pipefail

RNR_SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
RNR_IS_SOURCED=false

if [[ "$0" != "${BASH_SOURCE[0]}" ]]; then
	RNR_IS_SOURCED=true
	CM_QUIET=true
fi

source $RNR_SCRIPT_DIR/common/common.sh
source $RNR_SCRIPT_DIR/common/log.sh
source $RNR_SCRIPT_DIR/common/exp.sh
source $RNR_SCRIPT_DIR/scaleway/scaleway.sh

rnr_help() {
	local help_page=$(
		cat <<'EOF'
runner.sh -- AI trainer runner 
EOF
	)

	log_info "$help_page"
}

rnr_run_on() {
	local commit_sha repo_path exp_config_path exp_vars upload_tmpdir \
		real_upload_path real_repo_path ssh_jobs_arr artifacts_tmpdir \
		worker_artifacts_path compressed_artifacts_path

	commit_sha="$1"
	repo_path="$2"

	log_ok "Running workload @ ${2}#${1}"

	log_wait "Locking"
	exec 200> "$RNR_SCRIPT_DIR/lock"
	flock -n 200
	log_ok "Locked"

	log_wait "Loading experiment config"

	exp_config_path="${repo_path}/experiment.exp"

	if [ ! -f "$exp_config_path" ]; then
		log_err "No experiment.exp found"
		exit 1
	fi

	exp_vars="$(exp_load "$exp_config_path")"
	log_info "Experiment vars:\n$exp_vars"
	eval "$exp_vars"

	log_ok "Loaded experiment"

	upload_tmpdir=$(mktemp -d)
	log_wait "Preparing upload archive @ $upload_tmpdir"

	mkdir -p "$upload_tmpdir"

	real_repo_path=$(realpath "$repo_path")

	for upload in "${exp_uploads[@]}"; do
		log_wait "Adding $upload to archive"
		
		real_upload_path=$(realpath "${real_repo_path}/${upload}" \
			2>/dev/null || echo "failed")
		if [[ "$real_upload_path" == "failed" ]]; then
			log_err "Failed to resolve file $upload"
			return 1
		fi

		log_wait "Resolved real path @ $real_upload_path"

		if [[ "$real_upload_path" == "$real_repo_path"* ]]; then
			if [ -e "$real_upload_path" ]; then
				cp -r "$real_upload_path" "$upload_tmpdir"
				log_ok "Added '$upload' to archive"
			else
				log_err "'$upload' not found"
				return 1
			fi

		else
			log_err "File '$upload' is not in the repo"
			return 1
		fi
	done

	log_ok "Finalized upload dir"
	ls -lah "$upload_tmpdir"
	log_wait "Compressing"

	local archive_path="${upload_tmpdir}.tar.gz"
	tar czvf "$archive_path" "$upload_tmpdir"
	log_ok "Archive created @ $archive_path"

	eval "$(scw_start "$exp_num_workers")"

	for i in "${!scw_nocidrs[@]}"; do
		log_wait "Uploading archive to [$(( i+1 ))]"
		scw_upload "${scw_nocidrs[i]}" "$archive_path" /home/cogshelium00
		log_ok "Archive uploaded [$(( i+1 ))]"
	done

	ssh_jobs_arr=()

	for i in "${!scw_nocidrs[@]}"; do
		log_wait "Executing workflow on [$(( i+1 ))]"
		scw_ssh "${scw_nocidrs[i]}" "touch /home/cogshelium00/done && sleep 15" &
		ssh_jobs_arr+=($!)
	done

	wait "${ssh_jobs_arr[@]}"
	log_ok "Workflow complete on all workers"

	log_wait "Downloading artifacts"
	artifacts_tmpdir=$(mktemp -d)

	for i in "${!scw_nocidrs[@]}"; do
		worker_artifacts_path="${artifacts_tmpdir}/${i}"
		mkdir -p "$worker_artifacts_path"
		scw_download "${scw_nocidrs[i]}" "/home/cogshelium00/done" "$worker_artifacts_path" || log_err "Failed to download [$i]"
	done

	log_ok "Artifacts downloaded"

	log_wait "Compressing artifacts"
	compressed_artifacts_path="/tmp/${commit_sha}.tar.gz"
	tar czvf "$compressed_artifacts_path" "$artifacts_tmpdir"
	log_ok "Archive created @ $compressed_artifacts_path"

	log_wait "Removing original artifacts dir"
	rm -rf "$artifacts_tmpdir"
	log_ok "Original artifacts dir removed"

	scw_stop "${scw_wg_public_keys[@]}"
	log_ok "Instances stopped"
}

rnr_cli() {
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
		help|run_on)	
			local prefixed_command="rnr_${command}"
			$prefixed_command $args
			;;
		*)
			echo "ERROR: Unknown command"
			;;
	esac
}

if ! $RNR_IS_SOURCED; then
	rnr_cli $@
fi
