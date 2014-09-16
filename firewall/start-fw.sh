#!/bin/bash

source vars.sh

echo "Starting iptables firewall..."

# Add some VPN related rules
iptables -t nat -A INPUT -i eth0 -p udp -m udp --dport 1194 -j ACCEPT
iptables -t nat -A POSTROUTING -s ${vpnConnection} -o eth0 -j SNAT --to-source ${myLocalIP}

# Allow incoming ssh and ftp connections from allowed sources 
iptables -A INPUT -p tcp --dport 22 -s ${ssh_in} -j ACCEPT
iptables -A INPUT -p tcp --dport 21 -s ${ssh_in} -j ACCEPT

# Drop other incoming ssh and ftp connections
iptables -A INPUT -p tcp --dport 22 -j DROP
iptables -A INPUT -p tcp --dport 21 -j DROP

iptables -L

echo "Done"
