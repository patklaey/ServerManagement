#!/bin/bash

# Set the Viedo permission
find /home/pat/Videos/ -type f -exec chmod 640 {} \; -exec chown pat:pat {} \;
find /home/pat/Videos/ -type d -exec chmod 770 {} \; -exec chown pat:pat {} \;

# Set the Music permission
find /home/pat/Music/ -type f -exec chmod 640 {} \; -exec chown pat:pat {} \;
find /home/pat/Music/ -type d -exec chmod 770 {} \; -exec chown pat:pat {} \;

# Set the Pictures permission
find /home/pat/Pictures/ -type f -exec chmod 640 {} \; -exec chown pat:www-data {} \;
find /home/pat/Pictures/ -type d -exec chmod 770 {} \; -exec chown pat:www-data {} \;
