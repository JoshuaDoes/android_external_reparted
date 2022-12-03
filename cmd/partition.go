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

func (part *Partition) GetSize() int64 {
	size, err := humanize.ParseBytes(*part.Size)
	if err != nil {
		return -1
	}
	return int64(size)
}

//CheckValidOrPanic checks if the partition is valid, otherwise it panics
func (part *Partition) CheckValidOrPanic() {
	checkCount := part.Parted.SectorSizeLogical //Only check one logical sector
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
	if len(startingBytes) < checkCount {
		panic(fmt.Sprintf("Failed to read %d bytes from partition %d at offset %d, got %d bytes instead", checkCount, *part.Number, checkOffset, len(startingBytes)))
	}

	diskOffset := *part.Start + checkOffset
	diskBytes, err := part.Parted.ReadDisk(diskOffset, checkCount)
	if err != nil {
		panic(fmt.Sprintf("Failed to read %d bytes from disk at offset %d: %v", *part.Start, diskOffset, err))
	}
	if len(diskBytes) < checkCount {
		panic(fmt.Sprintf("Failed to read %d bytes from disk at offset %d, got %d bytes instead", checkCount, diskOffset, len(diskBytes)))
	}

	for j := int64(0); j < int64(checkCount); j++ {
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

func (part *Partition) Read(offset int64, num int) ([]byte, error) {
	if err := part.Open(); err != nil {
		return nil, err
	}
	defer part.Close()

	data := make([]byte, num)
	read, err := part.File.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if read < num {
		data = data[:read]
	}

	return data, nil
}