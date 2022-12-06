package main

import (
	"github.com/JoshuaDoes/json"

	"fmt"
	"io"
	"os"
	"os/exec"
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
	Disk string `json:"disk"`               //Path to raw disk device
	Parted string `json:"parted"`           //Path to parted executable
	Reserved []*Partition `json:"reserved"` //Partitions that must shrink/expand to fit new definitions
	UserData []*Partition `json:"userdata"` //Partitions that should dynamically readjust to leftover space
}

func (p *Parted) Run(args string) string {
	args = p.Config.Disk + " unit B " + args
	ret, err := exec.Command(p.Config.Parted, strings.Split(args, " ")...).Output()
	if err != nil {
		panic(fmt.Sprintf("Failed to call parted: %v", err))
	}

	return string(ret)
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
		return nil, err
	}
	if read < int(count) {
		data = data[:read]
	}

	return data, nil
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

func NewParted(pathJSON string) *Parted {
	partedJSON, err := os.ReadFile(pathJSON)
	if err != nil {
		panic(fmt.Sprintf("Failed to open JSON for reading from %s: %v", pathJSON, err))
	}

	partedCfg := &PartedConfig{}
	if err := json.Unmarshal(partedJSON, &partedCfg); err != nil {
		panic(fmt.Sprintf("Failed to load JSON from %s: %v", pathJSON, err))
	}

	if partedCfg.Disk == "" {
		panic(fmt.Sprintf("No disk specified"))
	}
	if partedCfg.Parted == "" {
		panic(fmt.Sprintf("No parted executable specified"))
	}
	for i := 0; i < len(partedCfg.Reserved); i++ {
		if partedCfg.Reserved[i].GetSize() <= 0 {
			panic(fmt.Sprintf("Invalid size specified for reserved partition %d", i+1))
		}
		if *partedCfg.Reserved[i].Name == "" && *partedCfg.Reserved[i].Number == 0 {
			panic(fmt.Sprintf("Must specify either name or number for reserved partition %d", i+1))
		}
	}
	for i := 0; i < len(partedCfg.UserData); i++ {
		if *partedCfg.UserData[i].Name == "" && *partedCfg.UserData[i].Number == 0 {
			panic(fmt.Sprintf("Must specify either name or number for userdata partition %d", i+1))
		}
	}

	p := &Parted{Config: partedCfg, Partitions: make([]*Partition, 0), HeaderSizes: make(map[string]int)}

	raw, err := os.Open(p.Config.Disk)
	if err != nil {
		panic(fmt.Sprintf("Failed to open disk %s: %v", p.Config.Disk, err))
	}
	p.File = raw

	partsWithFree := strings.Split(p.PrintFree(), "\n")
	header := true
	for i := 0; i < len(partsWithFree); i++ {
		line := partsWithFree[i]
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
						panic(fmt.Sprintf("Failed to scan size of disk %s: %v", p.Config.Disk, err))
					}
				case "Sector size (logical/physical)":
					_, err = fmt.Sscanf(val, "%dB/%dB", &p.SectorSizeLogical, &p.SectorSizePhysical)
					if err != nil {
						panic(fmt.Sprintf("Failed to scan logical/physical sector size of disk %s: %v", p.Config.Disk, err))
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
			if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition number: %v", err)) }
			counter += p.HeaderSizes["Number"]
			_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["Start"]], "%dB", &partStart)
			if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition start: %v", err)) }
			counter += p.HeaderSizes["Start"]
			_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["End"]], "%dB", &partEnd)
			if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition end: %v", err)) }
			counter += p.HeaderSizes["End"]
			_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["Size"]], "%s", &partSize)
			if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition size: %v", err)) }
			counter += p.HeaderSizes["Size"]
			if counter+p.HeaderSizes["File system"] >= len(line) {
				partFS = string(line[counter:])
			} else {
				_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["File system"]], "%s", &partFS)
				if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition filesystem: %v", err)) }
				counter += p.HeaderSizes["File system"]
				_, err = fmt.Sscanf(line[counter:counter+p.HeaderSizes["Name"]], "%s", &partName)
				if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition name: %v", err)) }
				counter += p.HeaderSizes["Name"]
				_, err = fmt.Sscanf(line[counter:], "%s", &partFlags)
				if err != nil && err != io.EOF { panic(fmt.Sprintf("Failed to scan partition flags: %v", err)) }
			}

			part := NewPartition(p, partNum, partStart, partEnd, partSize, partFS, partName, partFlags)
			realSize := part.GetSize()
			if realSize < 0 {
				panic(fmt.Sprintf("Failed to parse partiton size %s", partSize))
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
		panic(fmt.Sprintf("Parsed disk size, %d, is %d bytes less than counted partition sizes, %d - parted must be out of touch", p.DiskSize, p.TableSize * -1, p.PartsSize))
	}

	return p
}

func (p *Parted) Version() string {
	return p.Run("--version")
}

func (p *Parted) Help() string {
	return p.Run("--help")
}

func (p *Parted) PrintList(all bool) string {
	if all {
		return p.Run("print all")
	}
	return p.Run("print list")
}

func (p *Parted) PrintFree() string {
	return p.Run("print free")
}