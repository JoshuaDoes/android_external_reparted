//
// reparted orchestrates the application of a dynamic partition configuration when required
//
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/JoshuaDoes/json"
)

func init() {
	wd := filepath.Dir(os.Args[0])
	if err := os.Chdir(filepath.Dir(os.Args[0])); err != nil {
		fatal("Failed to change to working directory %s: %v", wd, err)
	}
}

var (
	logPrefix = "[reparted] "
)

func main() {
	p := NewParted(filepath.Base(os.Args[0]) + ".json")
	log("Loaded parted for disk " + p.Config.Disk)
	defer p.Close()

	for i := 0; i < len(p.Partitions); i++ {
		partJSON, err := json.Marshal(p.Partitions[i], false)
		if err == nil {
			log(string(partJSON))
		}
	}

	log("Disk model: %s", p.DiskModel)
	log("Disk total size: %d (%d logical / %d physical)", p.DiskSize, p.SectorSizeLogical, p.SectorSizePhysical)
	log("Disk flags: %s", p.DiskFlags)
	log("Partition table: %s", p.PartitionTable)
	log("Size of partition table: %d (partitions: %d)", p.TableSize, p.PartsSize)

	reserve := int64(0)
	for i := 0; i < len(p.Config.Reserved); i++ {
		reservePart := p.Config.Reserved[i]
		reserveSize := reservePart.GetSize()
		reserve += reserveSize

		actualPart := p.GetPartition(reservePart)
		if actualPart == nil {
			fatal("Reserved partition %d could not be matched to disk", i+1)
		}
		actualSize := actualPart.GetSize()
		if actualSize < reserveSize {
			reserve -= actualSize //Subtract only the amount that's already reserved
		} else if actualSize >= reserveSize {
			reserve -= actualSize //Subtract the full amount that's already reserved
		}
	}
	if reserve > 0 {
		log("Need to reserve %s/%s for new partition table", bytes(reserve), bytes(p.DiskSize))
	} else if reserve < 0 {
		log("Need to free %s/%s for new partition table", bytes(reserve * -1), bytes(p.DiskSize))
	} else {
		log("No additional space will be freed or reserved for new partition table")
	}
}

func bytes(num int64) string {
	return humanize.Bytes(uint64(num))
}

func log(msg ...interface{}) {
	if len(msg) > 0 {
		fmt.Printf(logPrefix)
		if len(msg) > 1 {
			fmt.Printf(msg[0].(string) + "\n", msg[1:]...)
		} else {
			fmt.Println(msg[0].(string))
		}
	}
}

func fatal(msg ...interface{}) {
	log("!!!FATAL!!!")
	log(msg...)
	os.Exit(1)
}
