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
	// Change the working directory to the directory in which the program is located.
	wd := filepath.Dir(os.Args[0])
	if err := os.Chdir(filepath.Dir(os.Args[0])); err != nil {
		fatal("Failed to change to working directory %s: %v", wd, err)
	}
}

var (
	logPrefix = "[reparted] " // A log prefix to be used for all log messages.
)

func main() {
	// Create a new Parted struct and initialize it with configuration data from a JSON file.
	p := NewParted(filepath.Base(os.Args[0]) + ".json")
	log("Loaded parted for disk " + p.Config.Disk)
	defer p.Close()

	// Print out information about the disk and its partitions.
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

	// Calculate the total amount of space that needs to be reserved for the new partition table.
	reserve := int64(0)
	for i := 0; i < len(p.Config.Reserved); i++ {
		// Add the expected reservation to the total reserve size.
		reservePart := p.Config.Reserved[i]
		reserveSize := reservePart.GetSize()
		reserve += reserveSize

		// Check if the reserved partition matches an existing partition on the disk.
		actualPart := p.GetPartition(reservePart)
		if actualPart == nil {
			fatal("Reserved partition %d could not be matched to disk", i+1)
		}

		// Remove the actual size of the existing partition from the total reserve count.
		// If reserve is 300MiB and actual is 400MiB, subtracting 400MiB results in -100MiB.
		actualSize := actualPart.GetSize()
		reserve -= actualSize // Subtract only the amount that's already reserved.
	}

	// Calculate space to be freed or reserved for new partition table.
	// A positive reserve size is the size that will be taken from userdata.
	// A negative reserve size is the size that will be awarded to userdata.
	if reserve > 0 {
		log("Need to reserve %s/%s for new partition table", bytes(reserve), bytes(p.DiskSize))
	} else if reserve < 0 {
		log("Need to free %s/%s for new partition table", bytes(reserve * -1), bytes(p.DiskSize))
	} else {
		log("No additional space will be freed or reserved for new partition table")
	}
}

// Convert a number of bytes to a human-readable string.
func bytes(num int64) string {
	return humanize.Bytes(uint64(num))
}

// Print a log message.
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

// Print a fatal error message and exit the program.
//
// This function is similar to the log() function, but it also prints the
// "!!!FATAL!!!" log message and exits the program with a non-zero exit code.
func fatal(msg ...interface{}) {
	log("!!!FATAL!!!")
	log(msg...)
	os.Exit(1)
}
