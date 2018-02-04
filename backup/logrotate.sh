#!/bin/bash
folderName=`date +'%b-%Y'`

tar -zcvf ${folderName}-schedule.tar.gz /root/idrive/Backup/Scheduled/LOGS/*
tar -zcvf ${folderName}-manual.tar.gz /root/idrive/Backup/Manual/LOGS/*

rm /root/idrive/Backup/Scheduled/LOGS/*
rm /root/idrive/Backup/Manual/LOGS/*