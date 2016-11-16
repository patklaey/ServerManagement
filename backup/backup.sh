#!/bin/bash

script_dir=`dirname $0`
source ${script_dir}/vars.sh

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

# Save the owncloud directory
echo -e "\nSaving owncloud directory..."
rsync --verbose --archive -h /home/owncloud /mnt/backup/home/

# Save wiki database
echo -e "\nSaving owncloud database..."
mysqldump -u ${OWNCLOUD_DB_USER} -p"${OWNCLOUD_DB_PASSWORD}" --opt --quote-names --skip-set-charset --default-character-set=latin1 ${OWNCLOUD_DATABASE} > /mnt/backup/owncloud-utf.sql
cp -p /mnt/backup/owncloud-utf.sql /var/tmp/owncloud-utf.sql

# Save the html directory
echo -e "\nSaving html directory..."
rsync --verbose --archive -h /home/html /mnt/backup/home/

# Save all pictures
echo -e "\nSaving pictures..."
rsync --verbose --archive -h /home/pat/Pictures /mnt/backup/home/pat/

# Save wiki database
echo -e "\nSaving wiki database..."
mysqldump -u ${WIKI_DB_USER} -p"${WIKI_DB_PASSWORD}" --opt --quote-names --skip-set-charset --default-character-set=latin1 ${WIKI_DATABASE} > /mnt/backup/wiki-utf.sql
cp -p /mnt/backup/wiki-utf.sql /var/tmp/wiki-utf.sql

# Save blog database
echo -e "\nSaving blog database..."
mysqldump -u ${WORDPRESS_DB_USER} -p"${WORDPRESS_DB_PASSWORD}" --opt --quote-names --skip-set-charset --default-character-set=latin1 ${WORDPRESS_DATABASE} > /mnt/backup/wordpress-utf.sql
cp -p /mnt/backup/wordpress-utf.sql /var/tmp/wordpress-utf.sql

# Save blog media
echo -e "\nSaving blog media..."
rsync --verbose --archive -h /home/blog-uploads /mnt/backup/home/

# Save config data
echo -e "\nSaving /etc..."
rsync --verbose --archive -h /etc /mnt/backup/config/

# Save webapps
echo -e "\nSaving webapps..."
rsync --verbose --archive -h /var/www /mnt/backup/config/

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

