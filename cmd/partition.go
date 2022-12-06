package main

import (
	"github.com/dustin/go-humanize"

	"fmt"
	"io"
	"os"
)

type Partition struct {
	Parted *Parted `json:"-"`
	Number *int `json:"num,omitempty"`
	Start *int64 `json:"start,omitempty"`
	End *int64 `json:"end,omitempty"`
	Size *string `json:"size,omitempty"` //1MiB, 1MB, 1KiB, 1KB, etc
	FS *string `json:"fs,omitempty"`
	Name *string `json:"name,omitempty"`
	Flags *string `json:"flags,omitempty"`
	File *os.File `json:"-"`

	Wipe bool `json:"wipe"` //Prevents running fsck and resize operations
}

func NewPartition(parted *Parted, num int, start, end int64, size, fs, name, flags string) *Partition {
	return &Partition{
		Parted: parted,
		Number: &num,
		Start: &start,
		End: &end,
		Size: &size,
		FS: &fs,
		Name: &name,
		Flags: &flags,
	}
}

func (part *Partition) Unmount() {
	partActual := part.Parted.GetPartition(false, part)
	if partActual == nil {
		return
	}
	_, _ = Run("umount", partActual.GetPath())
}

func (part *Partition) Resize() error {
	part.Unmount()
	if !part.Wipe {
		return nil
	}

	partActual := part.Parted.GetPartition(false, part)
	if partActual == nil {
		return fmt.Errorf("resize: Actual partition %s not found", part.GetName())
	}
	_, err := Run(part.Parted.Config.Resize, partActual.GetPath())
	if err != nil {
		return fmt.Errorf("resize %s: %v", partActual.GetPath(), err)
	}
	return nil
}

func (part *Partition) Fsck() error {
	part.Unmount()
	if !part.Wipe {
		return nil
	}

	partActual := part.Parted.GetPartition(false, part)
	if partActual == nil {
		return fmt.Errorf("fsck: Actual partition %s not found", part.GetName())
	}
	_, err := Run(part.Parted.Config.Fsck, partActual.GetPath())
	if err != nil {
		return fmt.Errorf("fsck %s: %v", partActual.GetPath(), err)
	}
	return nil
}

func (part *Partition) GetPath() string {
	return fmt.Sprintf("%s%d", part.Parted.Config.Disk, *part.Number)
}

func (part *Partition) GetName() string {
	return *part.Name
}

func (part *Partition) GetSize() int64 {
	size, err := humanize.ParseBytes(*part.Size)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse size: %v", err))
	}
	return int64(size)
}

func (part *Partition) GetSizeHuman() string {
	if part.Size == nil {
		size := humanize.Bytes(0)
		return size
	}

	//Rewrite the human size into the largest byte size
	size := part.GetSize()
	sizeHuman := humanize.Bytes(uint64(size))

	return sizeHuman
}

//CheckValidOrPanic checks if the partition is valid, otherwise it panics
func (part *Partition) CheckValidOrPanic() {
	checkCount := int64(part.Parted.SectorSizeLogical) //Only check one logical sector
	checkOffset := int64(0) //Check from the beginning of the partition

	if *part.Number == 0 || *part.FS == "Free Space" {
		return
	}

	if *part.End+1 - *part.Start != part.GetSize() {
		panic(fmt.Sprintf("Size for partition %d doesn't match end-start", *part.Number))
	}

	startingBytes, err := part.Read(checkOffset, checkCount)
	if err != nil {
		panic(fmt.Sprintf("Failed to read %d bytes from partition %d at offset %d: %v", checkCount, *part.Number, checkOffset, err))
	}
	if len(startingBytes) < int(checkCount) {
		panic(fmt.Sprintf("Failed to read %d bytes from partition %d at offset %d, got %d bytes instead", checkCount, *part.Number, checkOffset, len(startingBytes)))
	}

	diskOffset := *part.Start + checkOffset
	diskBytes, err := part.Parted.ReadDisk(diskOffset, checkCount)
	if err != nil {
		panic(fmt.Sprintf("Failed to read %d bytes from disk at offset %d: %v", *part.Start, diskOffset, err))
	}
	if len(diskBytes) < int(checkCount) {
		panic(fmt.Sprintf("Failed to read %d bytes from disk at offset %d, got %d bytes instead", checkCount, diskOffset, len(diskBytes)))
	}

	for j := int64(0); j < checkCount; j++ {
		if startingBytes[j] != diskBytes[j] {
			panic(fmt.Sprintf("Byte %d on partition %d does not match byte %d on disk, %d != %d", checkOffset + j, *part.Number, diskOffset + j, startingBytes[j], diskBytes[j]))
		}
	}
}

func (part *Partition) Open() error {
	if part.File != nil {
		return nil
	}

	partPath := fmt.Sprintf("%s%d", part.Parted.Config.Disk, *part.Number)
	raw, err := os.Open(partPath)
	part.File = raw
	return err
}

func (part *Partition) Close() error {
	if part.File != nil {
		if err := part.File.Close(); err != nil {
			return err
		}
		part.File = nil
	}
	return nil
}

func (part *Partition) Read(offset int64, count int64) ([]byte, error) {
	if err := part.Open(); err != nil {
		return nil, err
	}
	defer part.Close()

	data := make([]byte, count)
	read, err := part.File.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if read < int(count) {
		data = data[:read]
	}

	return data, nil
}