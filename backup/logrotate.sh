#!/bin/bash
folderName=`date +'%b-%Y'`

tar -zcvf /var/log/idrive/${folderName}-schedule.tar.gz /root/idrive/Backup/Scheduled/LOGS/*
tar -zcvf /var/log/idrive/${folderName}-manual.tar.gz /root/idrive/Backup/Manual/LOGS/*

rm /root/idrive/Backup/Scheduled/LOGS/*
rm /root/idrive/Backup/Manual/LOGS/*