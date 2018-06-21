#!/bin/sh

nvme id-ctrl -v $1 | grep ^sn | awk '{print $3}' | sed -e 's/vol/vol-/'
