#!/bin/bash

set -e

sudo apt update
sudo apt full-upgrade -y

sudo apt install -y openjdk-${java_version}-jre-headless nvme-cli jq curl

sudo mv /tmp/minemanagerd /usr/bin/minemanagerd
sudo chown root:root /usr/bin/minemanagerd
sudo chmod 755 /usr/bin/minemanagerd

sudo mv /tmp/minemanagerd.service /lib/systemd/system/minemanagerd.service
sudo chown root:root /lib/systemd/system/minemanagerd.service
sudo chmod 644 /lib/systemd/system/minemanagerd.service
sudo systemctl enable minemanagerd.service

sudo mv /tmp/mounter /usr/bin/mounter
sudo chown root:root /usr/bin/mounter
sudo chmod 4755 /usr/bin/mounter

sudo mv /tmp/unmounter /usr/bin/unmounter
sudo chown root:root /usr/bin/unmounter
sudo chmod 4755 /usr/bin/unmounter

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
for configfile in ops.json whitelist.json banned-players.json banned-ips.json server.properties; do
    ln -s /ebs/spotcraft/$configfile /minecraft/$configfile
done

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
max_ram_userdata=$(curl http://169.254.169.254/latest/user-data | jq '.max_ram')
if [ "\$max_ram_userdata" != "null"; then
   export MAX_RAM="\${max_ram_userdata}"
fi
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

mc_max_ram="${default_ram}"
max_ram_userdata=$(curl http://169.254.169.254/latest/user-data | jq '.max_ram')
if [ "\$max_ram_userdata" != "null"; then
   mc_max_ram="\${max_ram_userdata}"
fi

java -Xmx\${mc_max_ram}M -Xms\${mc_max_ram}M -jar server.jar nogui
EOF
        ;;
    *)
        echo "Invalid server type \"$server_type\""
        exit 1
        ;;
esac
