#!/bin/bash

#set -Eeuo pipefail

EXP_SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" \
	&> /dev/null && pwd )

source $EXP_SCRIPT_DIR/log.sh

__exp_key_count() {
	local config_path="$1"
	local config_key="$2"
	local count

	count=$(grep -c "$config_key" "$config_path" || true)
	echo "$count"
}

__exp_load_arr() {
	local config_path="$1"
	local config_key="$2"
	local arr_lines arr

	arr_lines=$(cat "$config_path" | grep "$config_key" | cut -d" " -f2)
	readarray -t arr < <(echo "$arr_lines")
	echo "${arr[@]@Q}"
}

__exp_load_key() {
	local config_path="$1"
	local config_key="$2"
	local default_value="$3"
	local key_count value

	key_count=$(__exp_key_count "$config_path" "$config_key")
	
	if [[ "$key_count" == "0" ]] && [[ "$default_value" != "" ]]; then
		echo "$default_value"
		return 0
	elif [[ "$key_count" != "1" ]]; then
		return 1
	fi

	value=$(cat "$config_path" | grep "$config_key" | cut -d" " -f2)
	echo "$value"
}

__exp_load_single() {
	local config_path="$1"
	local config_key="$2"
	local key_count single

	single=$(__exp_load_key "$config_path" "$config_key" "")
	echo "$single"
}

__exp_load_optional() {
	local config_path="$1"
	local config_key="$2"
	local default_value="$3"
	local key_count optional

	optional=$(__exp_load_key \
		"$config_path" "$config_key" "$default_value")
	echo "$optional"
}

exp_load() {
	local path local_prefix
	local use_local="${2:-"yes"}"

	if (( "$#" < "1" )) || (( "$#" > "2" )); then
		log_err "Usage: <path> [use_local_prefix]"
		exit 1
	fi

	path="$1"
	local_prefix=""

	if [[ "$use_local" == "yes" ]]; then
		local_prefix="local "
	fi

	exp_on_premise=$(__exp_load_single "$path" "OnPremise")
	echo "${local_prefix}exp_on_premise=\"$exp_on_premise\""
	exp_worker_type=$(__exp_load_single "$path" "WorkerType")
	echo "${local_prefix}exp_worker_type=\"$exp_worker_type\""
	exp_num_workers=$(__exp_load_single "$path" "NumWorkers")
	echo "${local_prefix}exp_num_workers=\"$exp_num_workers\""
	exp_uploads_arr=$(__exp_load_arr "$path" "Upload")
	echo "${local_prefix}exp_uploads=(${exp_uploads_arr[@]})"
	exp_artifacts_arr=$(__exp_load_arr "$path" "Artifact")
	echo "${local_prefix}exp_artifacts=(${exp_artifacts_arr[@]})"
}
