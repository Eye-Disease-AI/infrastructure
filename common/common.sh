#!/bin/bash

CM_SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" \
	&> /dev/null && pwd )
CM_QUIET=${CM_QUIET:-false}

source $CM_SCRIPT_DIR/log.sh

cm_pushd() {
	local before_dir

	if $CM_QUIET; then
		pushd $@ >/dev/null
	else
		before_dir="$PWD"
		pushd $@ >/dev/null
		log_info "Entering '$@'"
	fi
}

cm_popd() {
	if $CM_QUIET; then
		popd $@ >/dev/null
	else
		log_info "Leaving directory '$(pwd)'"
		popd $@ >/dev/null
	fi
}
