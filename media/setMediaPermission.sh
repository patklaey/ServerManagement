#!/bin/bash

# Set the Viedo permission
find /home/pat/Videos/ -type f -exec chmod 640 {} \; -exec chown pat:stream {} \;
find /home/pat/Videos/ -type d -exec chmod 770 {} \; -exec chown pat:stream {} \;

# Set the Music permission
find /home/pat/Music/ -type f -exec chmod 640 {} \; -exec chown pat:stream {} \;
find /home/pat/Music/ -type d -exec chmod 770 {} \; -exec chown pat:stream {} \;

# Set the Music permission
find /home/pat/Pictures/ -type f -exec chmod 640 {} \; -exec chown pat:stream {} \;
find /home/pat/Pictures/ -type d -exec chmod 770 {} \; -exec chown pat:stream {} \;
