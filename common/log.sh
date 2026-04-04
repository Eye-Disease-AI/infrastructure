#!/bin/bash

LOG_SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" \
	&> /dev/null && pwd )
LOG_DISABLED=${LOG_DISABLED:-false}
LOG_SOCKPATH=${LOG_SOCKPATH:-"none"}
LOG_DEST=${LOG_DEST:-"stdout"}

source $LOG_SCRIPT_DIR/color.sh

__log_date() {
	date +"%F %T"
}

log_init() {
	if [[ "$LOG_SOCKPATH" == "none" ]]; then
		LOG_SOCKPATH="$(mktemp -d)/sock"
		mkfifo "$LOG_SOCKPATH"

		if [[ "$LOG_DEST" == "stdout" ]]; then
			tail -f "$LOG_SOCKPATH" &
		elif [[ "$LOG_DEST" == "stderr" ]]; then
			tail -f "$LOG_SOCKPATH" 1>&2 &
		fi
	fi
}

log_ok() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}$(__log_date) ${DEF}${GRN}[:)]${DEF} $@\n" > "$LOG_SOCKPATH"
	fi
}

log_wait() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}$(__log_date) ${DEF}${CYN}[..]${DEF} $@\n" > "$LOG_SOCKPATH"
	fi
}

log_err() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}$(__log_date) ${DEF}${RED}[:(]${DEF} $@\n" > "$LOG_SOCKPATH"
	fi
}

log_lookdown() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}$(__log_date) ${DEF}${CYN}[-v]${DEF} $@\n" > "$LOG_SOCKPATH"
	fi
}

log_lookup() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}$(__log_date) ${DEF}${CYN}[-^]${DEF} $@\n" > "$LOG_SOCKPATH"
	fi
}

log_info() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}$(__log_date) [--]${DEF} $@\n" > "$LOG_SOCKPATH"
	fi
}

log_init
