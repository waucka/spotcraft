#!/bin/sh

echo "ubuntu ALL=(ALL) NOPASSWD:/sbin/poweroff" > /etc/sudoers.d/90-cloud-init-users
