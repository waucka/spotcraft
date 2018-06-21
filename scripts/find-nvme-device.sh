#!/bin/bash

for devname in $(ls /dev/nvme?); do
    volname=$(get-nvme-volname $devname)
    if [ "$volname" = "$1" ]; then
        echo $devname
        exit 0
    fi
done

exit 1
