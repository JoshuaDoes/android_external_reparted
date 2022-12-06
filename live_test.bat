@echo off
cls

echo [*] Building reparted
pushd cmd & set GOOS=linux & set GOARCH=arm64 & set GO111MODULE=off & go build -o ..\bin\reparted .\ & popd

echo [*] Creating ramdisk
adb shell su -c umount /mnt/ramdisk & adb shell su -c mkdir -p /mnt/ramdisk & adb shell su -c mount -t tmpfs -o size=40M tmpfs /mnt/ramdisk & adb shell su -c chmod 777 /mnt/ramdisk

echo [*] Checking and deleting old install (warning: wipes /data/local/tmp/reparted)
adb shell su -c rm -rf /data/local/tmp/reparted/ & adb shell su -c rm -rf /mnt/ramdisk/reparted/

echo [*] Pushing reparted and dependencies
adb push bin /data/local/tmp/reparted & adb push reparted.json /data/local/tmp/reparted/

echo [*] Moving reparted to ramdisk
adb shell su -c mv /data/local/tmp/reparted/ /mnt/ramdisk/reparted/

echo [*] Marking reparted and dependencies as executable
adb shell su -c chmod +x /mnt/ramdisk/reparted/*

echo [*] Starting reparted
adb shell su -c /mnt/ramdisk/reparted/reparted