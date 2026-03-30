#!/bin/bash

LOG_SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" \
	&> /dev/null && pwd )
LOG_DISABLED=${LOG_DISABLED:-false}

source $LOG_SCRIPT_DIR/color.sh

log_ok() {
	if ! $LOG_DISABLED; then
		printf "${GRN}[:)]${DEF} $@\n"
	fi
}

log_wait() {
	if ! $LOG_DISABLED; then
		printf "${CYN}[..]${DEF} $@\n"
	fi
}

log_err() {
	if ! $LOG_DISABLED; then
		printf "${RED}[:(]${DEF} $@\n"
	fi
}

log_lookdown() {
	if ! $LOG_DISABLED; then
		printf "${CYN}[-v]${DEF} $@\n"
	fi
}

log_lookup() {
	if ! $LOG_DISABLED; then
		printf "${CYN}[-^]${DEF} $@\n"
	fi
}

log_info() {
	if ! $LOG_DISABLED; then
		printf "${DIM}${WHT}[--]${DEF} $@\n"
	fi
}
