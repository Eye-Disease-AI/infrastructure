#!/bin/bash

set -Eeuo pipefail

CM_QUIET=${CM_QUIET:-false}
WG_SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
WG_PEERS_DIR="$WG_SCRIPT_DIR/peers"
WG_PUBKEY_PATH="$WG_SCRIPT_DIR/public.key"
WG_PRIVKEY_PATH="$WG_SCRIPT_DIR/private.key"
WG_ENABLE_FORWARDING="no"
WG_IS_SOURCED=false

if [[ "$0" != "${BASH_SOURCE[0]}" ]]; then
	WG_IS_SOURCED=true
	CM_QUIET=true
fi

source $WG_SCRIPT_DIR/common/log.sh

if ! $CM_QUIET; then
	log_info "Script base path detected: $WG_SCRIPT_DIR"
	log_info "Using peers dir: $WG_PEERS_DIR"
fi

wg_help() {
	local help_page=$(
		cat <<'EOF'
wireguard.sh -- wireguard helper tool
* help
* start
* genkeys
* genpsk
* addpeer <peer_name> <pubkey> <allowedips> [psk]
* delpeer <peer_name>
* listpeers [all|normal|hot]
* hotcreate
* hotdel <pubkey>
* hotdelall
* picknewip
* stop
EOF
	)

	log_info "$help_page"
}

wg_start() {
	local intf_path="/sys/class/net/wg0"

	if [ -d "$intf_path" ]; then
		log_err "wg0 exists, wireguard appears to be started"
		exit 1
	fi

	log_wait "Enabling wireguard"

	wg_ensurekeys
	log_lookup "Keys ensured"
	ip link add dev wg0 type wireguard
	log_lookup "Interface added"
	ip addr add dev wg0 10.13.13.1/24
	log_lookup "IP address set up"
	wg set wg0 listen-port 51871 private-key $WG_PRIVKEY_PATH
	log_lookup "Private key added"
	log_ok "Interface set up"

	log_wait "Adding peers from @ $WG_PEERS_DIR"

	for peer_path in $WG_PEERS_DIR/*; do
		[ -d "${peer_path}" ] || continue
		log_wait "Adding $peer_path"

		local peer_pubkey=$(cat "$peer_path/pubkey")
		local peer_psk_file="$peer_path/psk"
		local peer_allowedips=$(cat "$peer_path/allowedips")

		wg set wg0 \
			peer $peer_pubkey \
			preshared-key $peer_psk_file \
			allowed-ips $peer_allowedips

		log_ok "$peer_path added"
	done

	log_ok "Peers added"

	ip link set dev wg0 up
	log_ok "Link brung up"

	if [[ "$WG_ENABLE_FORWARDING" == "yes" ]]; then
		iptables -I FORWARD -i wg0 -o wg0 -j ACCEPT
		log_ok "Forwarding packets on wg0 subnet enabled"
	fi

	log_ok "Wireguard enabled"
}

wg_ensurekeys() {
	if [ ! -f "$WG_PUBKEY_PATH" ] && [ ! -f "$WG_PRIVKEY_PATH" ]; then
		wg_genkeys
	fi
}

wg_genkeys() {
	if [ -f "$WG_PUBKEY_PATH" ] || [ -f "$WG_PRIVKEY_PATH" ]; then
		log_err "Keys are already generated"
		exit 1
	fi

	wg genkey | tee private.key | wg pubkey > public.key
}

wg_genpsk() {
	if [[ "$#" != "1" ]]; then
		log_err "Usage: <dest_path>"
		exit 1
	fi

	local dest_path="$1"

	old_umask=$(umask)
	umask 0077
	wg genpsk > "$dest_path"
	umask $old_umask
}

wg_addpeer() {
	if (( $# < 3 )) || (( $# > 4 )); then
		log_err "Usage: <peer_name> <pubkey> <allowedips> [psk]"
		exit 1
	fi

	local name="$1"
	local pubkey="$2"
	local allowedips="$3"
	local psk=${4:-null}

	local peer_dir="${WG_PEERS_DIR}/${name}"
	local peer_pubkey_file="${peer_dir}/pubkey"
	local peer_psk_file="${peer_dir}/psk"
	local peer_allowedips_file="${peer_dir}/allowedips"

	if [ -d "$peer_dir" ]; then
		log_err "Peer using this name ($name) is already added"
		exit 1
	fi

	mkdir -p "$peer_dir"

	if [[ "$psk" == "null" ]]; then
		wg_genpsk "$peer_psk_file"
	else
		echo "$psk" > "$peer_psk_file"
	fi

	echo "$pubkey" > "$peer_pubkey_file"
	echo "$allowedips" > "$peer_allowedips_file"

	log_ok "Peer added"
}

wg_delpeer() {
	if [[ "$#" != "1" ]]; then
		log_err "Usage: <peer_name>"
		exit 1
	fi

	local name="$1"
	local peer_dir="${WG_PEERS_DIR}/${name}"

	if [ ! -d "$peer_dir" ]; then
		log_err "Peer '$name' does not exist"
		exit 1
	fi

	rm -r "$peer_dir"

	log_ok "Peer deleted"
}

__wg_listpeers_normal() {
	for peer_path in $WG_PEERS_DIR/*; do
		[ -d "${peer_path}" ] || continue
		cat "${peer_path}/pubkey"
	done
}

wg_listpeers() {
	local peer_type=${1:-all}
	local normal_peers all_peers normal_peers

	if ! $CM_QUIET; then
		log_wait "Listing peers (mode '$peer_type')"
	fi

	if [[ "$peer_type" == "all" ]]; then
		wg show wg0 peers | sort
	elif [[ "$peer_type" == "normal" ]]; then
		normal_peers=$(__wg_listpeers_normal)
		printf "$normal_peers" | sort
	elif [[ "$peer_type" == "hot" ]]; then
		all_peers=$(CM_QUIET=true wg_listpeers all)
		normal_peers=$(CM_QUIET=true wg_listpeers normal)
		comm -23 <(echo "$all_peers") <(echo "$normal_peers")
	fi

	if ! $CM_QUIET; then
		log_ok "Peers listed"
	fi
}

__wg_picknewip() {
	local used_ips=$(wg show wg0 allowed-ips | cut -f2)

	for ip_no in {100..254}; do
		local ip_proposal="10.13.13.${ip_no}/32"
		local used_count=$(echo "$used_ips" | \
			grep --count $ip_proposal)
		
		if [[ "$used_count" == 0 ]]; then
			echo "$ip_proposal"
			return 0
		fi
	done

	return 1
}

__wg_hotnew() {
	local private_key public_key psk ip_addr

	private_key=$(wg genkey)
	public_key=$(echo "$private_key" | wg pubkey)
	psk=$(wg genpsk)
	ip_addr=$(__wg_picknewip)

	echo "local wg_hotnew_public_key=$public_key"
	echo "local wg_hotnew_private_key=$private_key"
	echo "local wg_hotnew_psk=$psk"
	echo "local wg_hotnew_ip_addr=$ip_addr"
}

# Marked as private, not to pass psk over the cmdline
__wg_hotadd() {
	local peer_pubkey peer_allowedips peer_psk

	if (( $# != 3 )); then
		log_err "Usage: <pubkey> <allowedips> <psk>"
		exit 1
	fi

	peer_pubkey="$1"
	peer_allowedips="$2"
	peer_psk="$3"

	log_wait "Adding hot peer $peer_pubkey @ $peer_allowedips"

	echo "$peer_psk" | wg set wg0 \
		peer $peer_pubkey \
		preshared-key /dev/stdin \
		allowed-ips $peer_allowedips

	log_ok "$peer_pubkey added"
}

# Really for testing only, because we don't get privkey
# so we can't use this peer in any way. The real script should
# just use hotadd and hotnew commands.
wg_hotcreate() {
	local peerip hotpeer pubkey privkey psk

	log_wait "Creating and adding a new hot peer"
	hotpeer=$(__wg_hotnew)
	eval "$hotpeer"
	log_lookup "Generated a new peer, adding it"
	__wg_hotadd "$wg_hotnew_public_key" "$wg_hotnew_ip_addr" "$wg_hotnew_psk"
	log_ok "Added a new hot peer"
}

wg_hotdel() {
	local pubkey

	if [[ "$#" != 1 ]]; then
		log_err "Usage: <pubkey>"
		exit 1
	fi

	pubkey="$1"
	log_wait "Deleting hot peer $pubkey"
	wg set wg0 peer $pubkey remove
	log_ok "Peer deleted"
}

wg_hotdelall() {
	local hot_peers

	log_wait "Deleting all hot peers"
	hot_peers=$(CM_QUIET=true wg_listpeers hot)

	if [[ "$hot_peers" == "" ]]; then
		log_ok "No hot peers found"
		return 0
	fi

	readarray -t hot_peers_arr <<< $(echo -ne "${hot_peers}")

	for peer in "${hot_peers_arr[@]}"; do
	    log_lookup "Deleting ${peer}"
	    wg set wg0 peer ${peer} remove
	done

	log_ok "Hot peers deleted"
}

wg_stop() {
	log_wait "Disabling wireguard..."
	ip link delete dev wg0
	log_ok "Wireguard disabled"

	if [[ "$WG_ENABLE_FORWARDING" == "yes" ]]; then
		iptables -D FORWARD -i wg0 -o wg0 -j ACCEPT
	fi

	log_ok "Disabled packet forwarding over wg0"
}

wg_cli() {
	numargs=$#

	if ! $WG_IS_SOURCED; then
		local sfname=$(basename "$0")
	else
		local sfname="none"
	fi

	if [[ "$sfname" == "start" ]]; then
		local command="start"
		local args=""
	elif [[ "$numargs" == "0" ]]; then
		local command="help"
		local args=""
	else
		local command="$1"
		shift
		local args="$@"
	fi


	case $command in
		help|start|genkeys|genpsk|addpeer|delpeer|listpeers|hotcreate\
			|picknewip|hotdel|hotdelall|stop)
			local prefixed_command="wg_${command}"
			$prefixed_command $args
			;;
		*)
			echo "ERROR: Unknown command"
			;;
	esac
}

if ! $WG_IS_SOURCED; then
	wg_cli $@
fi
