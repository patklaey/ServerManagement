#!/bin/bash

source vars.sh

echo "Restoring iptables firewall from ${SAVE_RULES_FILE} ..."

iptables-restore < ${SAVE_RULES_FILE} 

echo "Done"
