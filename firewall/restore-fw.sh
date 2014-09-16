#!/bin/bash

source vars.sh

echo "Restoring iptables firewall from /root/firewall/rules ..."

iptables-restore < ${SAVE_RULES_FILE} 

echo "Done"
