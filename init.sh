#!/bin/bash

set -e

sudo apt update
sudo apt full-upgrade

sudo apt install openjdk-${java_version}-jre-headless nvme-cli

sudo mv /tmp/minemanagerd /usr/bin/minemanagerd
sudo chown root:root /usr/bin/minemanagerd
sudo chmod 755 /usr/bin/minemanagerd

sudo mv /tmp/mounter /usr/bin/mounter
sudo chown root:root /usr/bin/mounter
sudo chmod 4755 /usr/bin/mounter

sudo mv /tmp/get-nvme-volname /usr/bin/get-nvme-volname
sudo chown root:root /usr/bin/get-nvme-volname
sudo chmod 755 /usr/bin/get-nvme-volname

sudo mv /tmp/find-nvme-device.sh /usr/bin/find-nvme-device.sh
sudo chown root:root /usr/bin/find-nvme-device.sh
sudo chmod 755 /usr/bin/find-nvme-device.sh

sudo mv /tmp/find-nvme-device /usr/bin/find-nvme-device
sudo chown root:root /usr/bin/find-nvme-device
sudo chmod 755 /usr/bin/find-nvme-device

sudo mkdir /minecraft
sudo chown ubuntu:ubuntu /minecraft
sudo mkdir /ebs
ln -s /ebs/world /minecraft/world

cd /minecraft

echo 'At this point, you are agreeing to the Minecraft EULA.'
cat <<EOF > eula.txt
#By changing the setting below to TRUE you are indicating your agreement to our EULA (https://account.mojang.com/documents/minecraft_eula).
#$(date)
eula=TRUE
EOF
case $server_type in
    "ftb")
        unzip -l /tmp/server
        rm /tmp/server
        cat <<EOF > settings-local.sh
export MAX_RAM="${default_ram}M"
EOF
        if [ -f FTBInstall.sh ]; then
            chmod +x FTBInstall.sh
            ./FTBInstall.sh
        fi
        ;;
    "vanilla")
        mv /tmp/server server.jar
        cat <<EOF > ServerStart.sh
#!/bin/sh

java -Xmx${default_ram}M -Xms${default_ram}M -jar server.jar nogui
EOF
        ;;
    *)
        echo "Invalid server type \"$server_type\""
        exit 1
        ;;
esac

mv /tmp/server.properties.$server_type server.properties
rm /tmp/server.properties.*
