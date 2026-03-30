#!/bin/bash

set -eEuo pipefail

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
COMMON_DIR="$SCRIPT_DIR/common"

source $COMMON_DIR/log.sh

PID_FILE_PATH="${SCRIPT_DIR}/dnsmasq.pid"

log_info "Script base path detected: $SCRIPT_DIR"

help() {
	help_page=$(
		cat <<'EOF'
dnsmasq.sh -- dnsmasq helper tool
EOF
	)

	echo "$help_page"
}

start() {
	log_wait "Enabling dnsmasq"

	local dnsmasq_pid=$(cat "$PID_FILE_PATH")
	if kill -0 $dnsmasq_pid &>/dev/null; then
		log_err "Already enabled"
		exit 1
	fi

	dnsmasq \
		-C "${SCRIPT_DIR}/dnsmasq.conf" \
		-x "$PID_FILE_PATH" \
		-8 "${SCRIPT_DIR}/dnsmasq.log"

	log_ok "Dnsmasq enabled"
}

stop() {
	log_wait "Disabling dnsmasq"

	if [ ! -f "$PID_FILE_PATH" ]; then
		log_err "Already disabled"
		exit 1
	fi

	local dnsmasq_pid=$(cat "$PID_FILE_PATH")

	if ! kill -0 $dnsmasq_pid &>/dev/null; then
		log_err "Already disabled"
		exit 1
	fi

	kill $dnsmasq_pid
	
	while kill -0 $dnsmasq_pid &>/dev/null; do
		log_wait "Waiting..."
		sleep 1
	done

	log_ok "Dnsmasq disabled"
}

restart() {
	log_wait "Restarting dnsmasq"
	stop
	start
	log_ok "Dnsmasq restarted"
}

numargs=$#
sfname=$(basename "$0")

if [[ "$sfname" == "start" ]]; then
	command="start"
	args=""
elif [[ "$numargs" == "0" ]]; then
	command="help"
	args=""
else
	command="$1"
	shift
	args="$@"
fi

case $command in
	help|start|stop|restart)
		$command $args
		;;
	*)
		echo "ERROR: Unknown command"
		;;
esac
