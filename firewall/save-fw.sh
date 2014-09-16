#!/bin/bash

source vars.sh

echo "Saving iptables firewall to /root/firewall/rules ..."

iptables-save > ${SAVE_RULES_FILE}
chmod 600 ${SAVE_RULES_FILE}

echo "Done"
