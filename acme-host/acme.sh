#!/bin/bash

set -eEuo pipefail

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
COMMON_DIR="$SCRIPT_DIR/common"

source "${SCRIPT_DIR}/secrets.sh"
source "${COMMON_DIR}/log.sh"

log_info "Script base path detected: $SCRIPT_DIR"

help() {
	help_page=$(
		cat <<'EOF'
acme.sh -- acme helper tool
EOF
	)

	echo "$help_page"
}

issue() {
	if [[ "$#" != "1" ]]; then
		log_err "Usage: <domain_name>"
		exit 1
	fi

	local domain_name="$1"

	log_wait "Issuing certificate for $domain_name"

	rc=0
	acme.sh \
		--issue \
		--dns dns_ovh \
		-d "$domain_name" || rc=$?

	if [ $rc -ne 0 ]; then
		log_err "Issue failed for $domain"
	fi

	log_ok "Certificate issued for $domain"
}

install() {
	if [[ "$#" != "2" ]]; then
		log_err "Usage: <domain_name> <dest_path_dir>"
		exit 1
	fi

	local domain_name="$1"
	local dest_path_dir="$2"

	log_info "Installing cert files for ${domain_name} @ ${dest_path_dir}"

	acme.sh \
		--install-cert \
		-d "$domain_name" \
		--cert-file "${dest_path_dir}/cert.pem" \
		--key-file "${dest_path_dir}/key.pem" \
		--fullchain-file "${dest_path_dir}/fullchain.pem"

	log_ok "Cert files for ${domain_name} installed @ ${dest_path_dir}"
}

doall() {
	domains=(
		"labelstudio.krzyzanowski.dev"
		"mlflow.krzyzanowski.dev"
	)

	for domain in "${domains[@]}"; do
		issue "$domain"
		local dest_dir="${SCRIPT_DIR}/certs/${domain}"
		mkdir -p "$dest_dir"
		install "$domain" "$dest_dir"
	done

	log_ok "All certs issued and installed"
}

numargs=$#

if [[ "$numargs" == "0" ]]; then
	command="help"
	args=""
else
	command="$1"
	shift
	args="$@"
fi

case $command in
	help|issue|install|doall)
		$command $args
		;;
	*)
		echo "ERROR: Unknown command"
		;;
esac
