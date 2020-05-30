#!/bin/bash
folderName=`date +'%b-%Y'`

tar -zcvf /var/log/idrive/${folderName}-scheduled.tar.gz /var/log/idrive/*_Scheduled
tar -zcvf /var/log/idrive/${folderName}-manual.tar.gz /var/log/idrive/*_Manual

rm /var/log/idrive/*_Scheduled
rm /var/log/idrive/*_Manual