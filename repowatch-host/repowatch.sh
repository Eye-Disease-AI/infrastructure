#!/bin/bash

set -Eeuo pipefail

REW_SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
REW_REPO_NAME="testing"
REW_REPO_PATH="${REW_SCRIPT_DIR}/${REW_REPO_NAME}"
#REW_REPO_URL="git@github.com:Eye-Disease-AI/testing.git"
REW_REPO_URL="$REW_SCRIPT_DIR/testing_src"
REW_REPO_LATEST_REV_PATH="$REW_SCRIPT_DIR/latest_rev"
REW_REPO_BRANCH=master
REW_IS_SOURCED=false

if [[ "$0" != "${BASH_SOURCE[0]}" ]]; then
	REW_IS_SOURCED=true
	CM_QUIET=true
fi

source $REW_SCRIPT_DIR/common/common.sh
source $REW_SCRIPT_DIR/common/log.sh

rew_help() {
	local help_page=$(
		cat <<'EOF'
repowatch.sh -- repo watcher tool
EOF
	)

	log_info "$help_page"
}

# Assumes repo exists
__rew_has_commits() {
	local glog

	pushd "$REW_REPO_PATH" 1>/dev/null
	glog=$(git log >/dev/null || echo "none")
	popd 1>/dev/null

	if [[ "$glog" != "none" ]]; then
		glog="some"
	fi

	echo "$glog"
}

__rew_get_run_commits() {
	local range_start=${1:-none}
	local range_end=${2:-none}

	if [[ "$range_start" != "none" ]] && \
	   [[ "$range_end" != "none" ]]; then
		minimal_log=$( \
			git log $range_start..$range_end \
			--pretty=format:"%H %s" \
		)
	else
		minimal_log=$(git log --pretty=format:"%H %s")
	fi

	grep "\[run\]" <<<"$minimal_log" || echo "none"
}

rew_init() {
	local has_commits minimal_log run_cmt latest_rev

	if [ ! -d "$REW_REPO_PATH" ]; then
		log_info "Repository not found"
		log_wait "Cloning repo"
		git clone "$REW_REPO_URL" "$REW_REPO_PATH" >/dev/null
		log_ok "Repo cloned @ $REW_REPO_PATH"
	else
		log_ok "Repository ($REW_REPO_PATH) found"
	fi

	has_commits=$(__rew_has_commits)

	if [[ "$has_commits" == "some" ]]; then
		log_info "Some commits found"
		log_wait "Looking for [run] commits"
		cm_pushd "$REW_REPO_PATH"
		run_cmt=$(__rew_get_run_commits)

		if [[ "$run_cmt" == "none" ]]; then
			log_info "No run commits found"
		else
			log_info "There are some [run] commits, if needed \
please trigger them manually"
		fi

		latest_rev=$(git rev-parse HEAD)
		cm_popd
	else
		log_info "None commits found, we can't really do anything"
		latest_rev="none"
	fi

	echo "$latest_rev" > latest_rev
	log_ok "Saved latest revision @ $latest_rev"
}

rew_update() {
	local new_latest_rev old_latest_rev fetch_res run_cmts lrc_cmt

	cm_pushd "$REW_REPO_PATH"

	log_wait "Fetching new changes"
	fetch_res=$(git fetch origin "$REW_REPO_BRANCH" || echo "fail")

	if [[ "$fetch_res" == "fail" ]]; then
		log_err "Failed to fetch changes"
		log_err "Make sure that repo is not empty and it has \
$REW_REPO_BRANCH branch"
		cm_popd
		return 1
	fi

	log_wait "Changes fetch, comparing with latest rev"
	new_latest_rev="$(git rev-parse origin/master)"
	old_latest_rev="$(cat $REW_REPO_LATEST_REV_PATH)"
	log_lookup "New rev: $new_latest_rev"
	log_lookup "Old rev: $old_latest_rev"

	if [[ "$old_latest_rev" == "none" ]]; then
		log_info "Old rev is none, not running anything for safety"
		echo "$new_latest_rev" > "$REW_REPO_LATEST_REV_PATH"
		log_ok "Latest rev updated = $new_latest_rev"
	elif [[ "$old_latest_rev" == "$new_latest_rev" ]]; then
		log_ok "Nothing new, nothing to be done"
	else
		log_info "There some new commits"
		git log --oneline ${old_latest_rev}..${new_latest_rev}
		log_info "Checking for [run] commits"
		run_cmts="$(__rew_get_run_commits \
			$old_latest_rev $new_latest_rev)"

		if [[ "$run_cmts" == "none" ]]; then
			log_info "No [run] commits found"
		else
			log_lookdown "Some [run] commits found"
			echo "$run_cmts"
			log_info "Picking least recent [run] commit"
			lrc_cmt=$(echo "$run_cmts" | tail -n1)
			lrc_cmt_sha=$(echo "$lrc_cmt" | cut -d " " -f1)

			log_wait "Resetting working directory @ $lrc_cmt_sha"
			(cd "$REW_REPO_PATH" && git reset --hard $lrc_cmt_sha)

			log_wait "Updating rev = $lrc_cmt_sha"
			echo "$lrc_cmt_sha" > "$REW_REPO_LATEST_REV_PATH"
			log_ok "Rev updated"

			log_wait "Running on the commit @ $lrc_cmt_sha"
			$REW_SCRIPT_DIR/../runner-host/runner.sh \
				run_on "$lrc_cmt_sha" "$REW_REPO_PATH"
			log_ok "Running finished"
		fi
	fi

	cm_popd
}

rew_cli() {
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
		help|init|update)	
			local prefixed_command="rew_${command}"
			$prefixed_command $args
			;;
		*)
			echo "ERROR: Unknown command"
			;;
	esac
}

if ! $REW_IS_SOURCED; then
	rew_cli $@
fi
