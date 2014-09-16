#!/bin/bash

source vars.sh

echo "Restarting iptables firewall ... "

${DIRECTORY}/stop-fw.sh

echo ""

${DIRECTORY}/start-fw.sh

echo "Done"
