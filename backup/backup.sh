#!/bin/bash

source vars.sh

echo "################################################################################"
echo "Starting backup"
date

# Mount the backup disk
mount --rw /dev/sdb1 /mnt/backup

mountExitCode=$?

if [[ ${mountExitCode} -ne 0 ]]; then
    echo "Mounting /dev/sdb1 failed: ${mountExitCode} stopping here"
    echo -e "\nBackup finished with errors"
    echo "################################################################################"
    exit 1;
fi

# Save the webdav directory
echo -e "\nSaving webdav directory..."
rsync --verbose --archive -h /home/webdav /mnt/backup/home/

# Save the html directory
echo -e "\nSaving html directory..."
rsync --verbose --archive -h /home/html /mnt/backup/home/

# Save all pictures
echo -e "\nSaving pictures..."
rsync --verbose --archive -h /home/pat/Pictures /mnt/backup/home/pat/

# Save blog database
echo -e "\nSaving blog database..."
mysqldump -u ${WORDPRESS_DB_USER} -p${WORDPRESS_DB_PASSWORD} --opt --quote-names --skip-set-charset --default-character-set=latin1 ${WORDPRESS_DATABASE} > /mnt/backup/wordpress-utf.sql

# Save blog media
echo -e "\nSaving blog media..."
rsync --verbose --archive -h /home/blog-uploads /mnt/backup/home/

# Write the server image
echo -e "\nSaving server image..."
time dd if=/dev/mmcblk0 of=/mnt/backup/server.img
sync

# As infortmaion show filesystem usage stats
echo -e "\n"
df -h

# Unmount the backup disk
umount /mnt/backup

echo -e "\nBackup finished"
date
echo "################################################################################"

