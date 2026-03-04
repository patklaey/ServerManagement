#!/bin/bash

script_dir=`dirname $0`
source ${script_dir}/vars.sh

echo "################################################################################"
echo "Starting backup"
date

# Mount the backup disk
mount --rw UUID=${BACKUP_DISK_UUID} /mnt/backup/

mountExitCode=$?

if [[ ${mountExitCode} -ne 0 ]]; then
    echo "Mounting /dev/sdb1 failed: ${mountExitCode} stopping here"
    echo -e "\nBackup finished with errors"
    echo "################################################################################"
    exit 1;
fi

# Create DB backup directory if it does not exist
mkdir -p /root/db_backup

# Save the owncloud directory
echo -e "\nSaving owncloud directory..."
rsync --verbose --archive -h /data/owncloud /mnt/backup/home/

# Save wiki database
echo -e "\nSaving owncloud database..."
docker exec owncloud-db sh -c "mysqldump -u ${OWNCLOUD_DB_USER} -p'${OWNCLOUD_DB_PASSWORD}' --opt --quote-names --skip-set-charset --default-character-set=latin1 ${OWNCLOUD_DATABASE} > /backup/owncloud-utf.sql"
cp -p /root/db_backup/owncloud-utf.sql /mnt/backup/owncloud-utf.sql

# Save all pictures
echo -e "\nSaving pictures..."
rsync --verbose --archive -h /home/pat/Pictures /mnt/backup/home/pat/

# Save wiki database
echo -e "\nSaving wiki database..."
docker exec mediawiki-db sh -c "mysqldump -u ${WIKI_DB_USER} -p'${WIKI_DB_PASSWORD}' --opt --quote-names --skip-set-charset --default-character-set=latin1 ${WIKI_DATABASE} > /backup/wiki-utf.sql"
cp -p /root/db_backup/wiki-utf.sql /mnt/backup/wiki-utf.sql

# Save blog database
echo -e "\nSaving blog database..."
docker exec wordpress-db sh -c "mysqldump -u ${WORDPRESS_DB_USER} -p'${WORDPRESS_DB_PASSWORD}' --opt --quote-names --skip-set-charset --default-character-set=latin1 ${WORDPRESS_DATABASE} > /backup/wordpress-utf.sql"
cp -p /root/db_backup/wordpress-utf.sql /mnt/backup/wordpress-utf.sql

# Save blog volumes
docker-compose -f /root/docker/docker-wordpress/docker-compose.yml stop
rsync --verbose --archive -h /data/docker-volumes/wordpress-database /mnt/backup/docker-volumes
docker-compose -f /root/docker/docker-wordpress/docker-compose.yml start

# Save blog media
echo -e "\nSaving blog media..."
rsync --verbose --archive -h /data/blog-uploads /mnt/backup/home/

# Save config data
echo -e "\nSaving /etc..."
rsync --verbose --archive -h /etc /mnt/backup/config/

# Save root data
echo -e "\nSaving /root..."
rsync --verbose --archive -h /root /mnt/backup/root/

sync

# As information show filesystem usage stats
echo -e "\n"
df -h

# Unmount the backup disk
umount /mnt/backup

echo -e "\nBackup finished"
date
echo "################################################################################"

