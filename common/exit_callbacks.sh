#!/bin/bash

cm_run_exit_callbacks() {
  for (( i=${#CM_EXIT_CALLBACKS}-1; i >= 0; i-- )); do
    eval "${CM_EXIT_CALLBACKS[$i]}"
  done
}

cm_add_exit_callback() {
  CM_EXIT_CALLBACKS+=("$*")
}

if [[ -z "${CM_EXIT_CALLBACKS:-}" ]]; then
  declare -a CM_EXIT_CALLBACKS=()
  trap cm_run_exit_callbacks EXIT
fi
