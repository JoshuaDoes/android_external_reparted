package main

import (
	"github.com/JoshuaDoes/json"

	"fmt"
	"io"
	"os"
	"strings"
)

// A struct representing a disk for parted.
type Parted struct {
	// The configuration data for the Parted struct.
	Config *PartedConfig

	// The disk model, such as "ATA VBOX HARDDISK".
	DiskModel string
	// The disk flags, such as "msftdata".
	DiskFlags string
	// The partition table type, such as "gpt".
	PartitionTable string
	// The logical sector size of the disk.
	SectorSizeLogical int64
	// The physical sector size of the disk.
	SectorSizePhysical int64
	// The total size of the disk in bytes.
	DiskSize int64
	// The size of the partition table in bytes.
	TableSize int64
	// The size of the partitions in bytes.
	PartsSize int64

	// The file descriptor for the disk.
	File *os.File
	// A map containing the sizes of various table headers from parted for table parsing.
	HeaderSizes map[string]int
	Partitions []*Partition
}

type PartedConfig struct {
	Parted string `json:"parted"` //Path to parted executable
	Fsck   string `json:"fsck"`   //Path to fsck executable (such as e2fsck)
	Resize string `json:"resize"` //Path to resize executable (such as resize2fs)

	Disk string `json:"disk"`               //Path to raw disk device
	Reserved []*Partition `json:"reserved"` //Partitions that must shrink/expand to fit new definitions
	UserData []*Partition `json:"userdata"` //Partitions that should dynamically readjust to leftover space
}

func (p *Parted) Run(args string) (string, error) {
	//-s --script: Prevents interactive prompts
	//-f --fix: Don't abort when asked interactive things
	//---pretend-input-tty: Undocumented way to allow scripting
	//unit B: Always use bytes instead of human-readable sizes
	args = "--script --fix " + p.Config.Disk + " ---pretend-input-tty unit B " + args
	return Run(p.Config.Parted, args)
}

func (p *Parted) Close() {
	for i := 0; i < len(p.Partitions); i++ {
		p.Partitions[i].Close()
	}

	p.File.Close()
}

func (p *Parted) ReadDisk(offset int64, count int64) ([]byte, error) {
	data := make([]byte, count)
	read, err := p.File.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("ReadDisk: Failed to read bytes from offset %d: %v", offset, err)
	}
	if read < int(count) {
		data = data[:read]
	}

	return data, nil
}

func (p *Parted) WriteDisk(offset int64, data []byte) error {
	if _, err := p.File.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("WriteDisk: Failed to seek to offset %d: %v", offset, err)
	}
	if _, err := p.File.Write(data); err != nil {
		return fmt.Errorf("WriteDisk: Failed to write bytes at offset %d: %v", offset, err)
	}
	return nil
}

func (p *Parted) GetPartition(reserved bool, match *Partition) *Partition {
	if match.Name != nil {
		return p.GetPartitionByName(reserved, *match.Name)
	}
	if match.Number != nil {
		return p.GetPartitionByNum(reserved, *match.Number)
	}
	return nil
}
func (p *Parted) GetPartitionByName(reserved bool, name string) *Partition {
	if reserved {
		for i := 0; i < len(p.Config.Reserved); i++ {
			if *p.Config.Reserved[i].Name == name {
				return p.Config.Reserved[i]
			}
		}
	} else {
		for i := 0; i < len(p.Partitions); i++ {
			if *p.Partitions[i].Name == name {
				return p.Partitions[i]
			}
		}
	}
	return nil
}
func (p *Parted) GetPartitionByNum(reserved bool, num int) *Partition {
	if reserved {
		for i := 0; i < len(p.Config.Reserved); i++ {
			if *p.Config.Reserved[i].Number == num {
				return p.Config.Reserved[i]
			}
		}
	} else {
		for i := 0; i < len(p.Partitions); i++ {
			if *p.Partitions[i].Number == num {
				return p.Partitions[i]
			}
		}
	}
	return nil
}
func (p *Parted) GetUserDataPartitions(reserved bool) []*Partition {
	if reserved {
		return p.Config.UserData
	}
	userData := make([]*Partition, 0)
	for i := 0; i < len(p.Config.UserData); i++ {
		if p.Config.UserData[i].Name != nil {
			part := p.GetPartitionByName(false, *p.Config.UserData[i].Name)
			if part != nil {
				userData = append(userData, part)
			}
		} else if p.Config.UserData[i].Number != nil {
			part := p.GetPartitionByNum(false, *p.Config.UserData[i].Number)
			if part != nil {
				userData = append(userData, part)
			}
		}
	}
	return userData
}

func NewParted(pathJSON string) (*Parted, error) {
	partedJSON, err := os.ReadFile(pathJSON)
	if err != nil {
		return nil, fmt.Errorf("Failed to open JSON for reading from %s: %v", pathJSON, err)
	}

	partedCfg := &PartedConfig{}
	if err := json.Unmarshal(partedJSON, &partedCfg); err != nil {
		return nil, fmt.Errorf("Failed to load JSON from %s: %v", pathJSON, err)
	}

	if partedCfg.Disk == "" {
		return nil, fmt.Errorf("No disk specified")
	}
	if partedCfg.Parted == "" {
		return nil, fmt.Errorf("No parted executable specified")
	}
	for i := 0; i < len(partedCfg.Reserved); i++ {
		if partedCfg.Reserved[i].GetSize() <= 0 {
			return nil, fmt.Errorf("Invalid size specified for reserved partition %d", i+1)
		}
		if *partedCfg.Reserved[i].Name == "" {
			return nil, fmt.Errorf("Must specify name for reserved partition %d", i+1)
		}
	}
	for i := 0; i < len(partedCfg.UserData); i++ {
		if *partedCfg.UserData[i].Name == "" {
			return nil, fmt.Errorf("Must specify name for userdata partition %d", i+1)
		}
	}

	p := &Parted{Config: partedCfg, Partitions: make([]*Partition, 0), HeaderSizes: make(map[string]int)}

	raw, err := os.Open(p.Config.Disk)
	if err != nil {
		return nil, fmt.Errorf("Failed to open disk %s: %v", p.Config.Disk, err)
	}
	p.File = raw

	partsWithFree, err := p.PrintFree()
	if err != nil {
		return nil, fmt.Errorf("Failed to get partition list: %v", err)
	}

	partsLines := strings.Split(partsWithFree, "\n")
	header := true
	for i := 0; i < len(partsLines); i++ {
		line := partsLines[i]
		if line == "" {
			continue
		}

		if header {
			mapping := strings.Split(line, ": ")
			if len(mapping) > 1 {
				val := mapping[1]
				switch mapping[0] {
				case "Model":
					p.DiskModel = val
				case "Disk " + p.Config.Disk:
					_, err = fmt.Sscanf(val, "%dB", &p.DiskSize)
					if err != nil {
						return nil, fmt.Errorf("Failed to scan size of disk %s: %v", p.Config.Disk, err)
					}
				case "Sector size (logical/physical)":
					_, err = fmt.Sscanf(val, "%dB/%dB", &p.SectorSizeLogical, &p.SectorSizePhysical)
					if err != nil {
						return nil, fmt.Errorf("Failed to scan logical/physical sector size of disk %s: %v", p.Config.Disk, err)
					}
				case "Partition Table":
					p.PartitionTable = strings.ToUpper(val)
				case "Disk Flags":
					p.DiskFlags = val
				}
			} else {
				//Parse partition table spacing header
				val := mapping[0]
				key := ""
				counter := 0
				for j := 0; j < len(val); j++ {
					switch val[j] {
					case ' ':
						if key == "File" {
							key += " "
						} else {
							counter++
						}
					default:
						if counter > 0 || val[j] == '\n' {
							p.HeaderSizes[key] = len(key) + counter
							counter = 0
							key = ""
						}
						key += string(val[j])
					}
				}
				header = false
			}
		} else {
			partNum := 0
			partStart := int64(0)
			partEnd := int64(0)
			partSize := ""
			partFS := ""
			partName := ""
			partFlags := ""

			counter := 0
			_, err := fmt.Sscanf(line[0:p.HeaderSizes["Number"]], "%d", &partNum)
			if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition number: %v", err) }
			counter += p.HeaderSizes["Number"]
			_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["Start"]], "%dB", &partStart)
			if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition start: %v", err) }
			counter += p.HeaderSizes["Start"]
			_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["End"]], "%dB", &partEnd)
			if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition end: %v", err) }
			counter += p.HeaderSizes["End"]
			_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["Size"]], "%s", &partSize)
			if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition size: %v", err) }
			counter += p.HeaderSizes["Size"]
			if counter+p.HeaderSizes["File system"] >= len(line) {
				partFS = string(line[counter:])
			} else {
				_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["File system"]], "%s", &partFS)
				if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition filesystem: %v", err) }
				counter += p.HeaderSizes["File system"]
				if counter+p.HeaderSizes["Name"] >= len(line) {
					partName = string(line[counter:])
				} else {
					_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["Name"]], "%s", &partName)
					if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition name: %v", err) }
					counter += p.HeaderSizes["Name"]
					_, err = fmt.Sscanf(line[counter:], "%s", &partFlags)
					if err != nil && err != io.EOF { return nil, fmt.Errorf("Failed to scan partition flags: %v", err) }
				}
			}

			part := NewPartition(p, partNum, partStart, partEnd, partSize, partFS, partName, partFlags)
			realSize := part.GetSize()
			if realSize < 0 {
				return nil, fmt.Errorf("Failed to parse partiton size %s", partSize)
			}

			p.Partitions = append(p.Partitions, part)
		}
	}

	for i := 0; i < len(p.Partitions); i++ {
		p.PartsSize += p.Partitions[i].GetSize()
		p.Partitions[i].CheckValidOrPanic()
	}

	p.TableSize = p.DiskSize - p.PartsSize
	if p.TableSize < 0 {
		return nil, fmt.Errorf("Parsed disk size, %d, is %d bytes less than counted partition sizes, %d - parted must be out of touch", p.DiskSize, p.TableSize * -1, p.PartsSize)
	}

	for i := 0; i < len(p.Config.Reserved); i++ {
		p.Config.Reserved[i].Parted = p
		partActual := p.GetPartition(false, p.Config.Reserved[i])
		if partActual != nil {
			if p.Config.Reserved[i].Wipe {
				partActual.Wipe = true
			}
		}
	}

	return p, nil
}

func (p *Parted) Help() (string, error) {
	return p.Run("--help")
}

func (p *Parted) MkPart(start, end int64) (string, error) {
	return p.Run(fmt.Sprintf("mkpart primary %d %d", start, end))
}

func (p *Parted) Name(num int, name string) (string, error) {
	return p.Run(fmt.Sprintf("name %d %s", num, name))
}

func (p *Parted) PrintFree() (string, error) {
	return p.Run("print free")
}

func (p *Parted) PrintList(all bool) (string, error) {
	if all {
		return p.Run("print all")
	}
	return p.Run("print list")
}

func (p *Parted) ResizePart(num int, end int64) (string, error) {
	actualPart := p.GetPartitionByNum(false, num)
	if actualPart == nil {
		return "", fmt.Errorf("parted: ResizePart: unable to find partition %d", num)
	}
	output, err := p.Rm(num)
	if err != nil {
		return output, fmt.Errorf("parted: ResizePart: failed to delete partition %d: %v", num, err)
	}
	output, err = p.MkPart(*actualPart.Start, *actualPart.End)
	if err != nil {
		return output, fmt.Errorf("parted: ResizePart: failed to create partition %d: %v", num, err)
	}
	output, err = p.Name(num, *actualPart.Name)
	if err != nil {
		return output, fmt.Errorf("parted: ResizePart: failed to name partition %d: %v", num, err)
	}
	output, err = p.Set(num, *actualPart.Flags, true)
	if err != nil {
		return output, fmt.Errorf("parted: ResizePart: failed to set flags for partition %d: %v", num, err)
	}

	return "", nil
}

func (p *Parted) Rm(num int) (string, error) {
	return p.Run(fmt.Sprintf("rm %d", num))
}

func (p *Parted) Set(num int, flag string, state bool) (string, error) {
	realState := "off"
	if state {
		realState = "on"
	}
	return p.Run(fmt.Sprintf("set %d %s %s", num, flag, realState))
}

func (p *Parted) Version() (string, error) {
	return p.Run("--version")
}