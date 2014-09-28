#!/bin/bash

echo "################################################################################"
echo "Starting backup"
date

# Mount the backup disk
mount --rw /dev/sdb1 /mnt/backup

# Save the webdav directory
echo -e "\nSaving webdav directory..."
rsync --verbose --archive -h /home/webdav /mnt/backup/home/

# Save the html directory
echo -e "\nSaving html directory..."
rsync --verbose --archive -h /home/html /mnt/backup/home/

# Save all pictures
echo -e "\nSaving pictures..."
rsync --verbose --archive -h /home/pat/Pictures /mnt/backup/home/pat/

# As infortmaion show filesystem usage stats
echo -e "\n"
df -h

# Unmount the backup disk
umount /mnt/backup

echo -e "\nBackup finished"
date
echo "################################################################################"

