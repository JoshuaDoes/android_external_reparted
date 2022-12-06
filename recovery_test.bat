@echo off
cls

echo [*] Building reparted
pushd cmd & go env -w GOOS=linux GOARCH=arm64 GO111MODULE=off & go build -o ..\bin\reparted .\ & popd

echo [*] Creating ramdisk
adb shell umount /mnt/ramdisk & adb shell mkdir -p /mnt/ramdisk & adb shell mount -t ramfs -o size=1M ramfs /mnt/ramdisk

echo [*] Checking and deleting old install (warning: wipes /mnt/ramdisk/reparted)
adb shell rm -rf /mnt/ramdisk/reparted/

echo [*] Pushing reparted and dependencies
adb push bin /mnt/ramdisk/reparted/ & adb push reparted.json /mnt/ramdisk/reparted/

echo [*] Marking reparted and dependencies as executable
adb shell chmod +x /mnt/ramdisk/reparted/*

echo [*] Starting reparted
adb shell /mnt/ramdisk/reparted/reparted