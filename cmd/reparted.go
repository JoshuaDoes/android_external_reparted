//
// reparted orchestrates the application of a dynamic partition configuration when required
//
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dustin/go-humanize"
	//"github.com/JoshuaDoes/json"
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
	p, err := NewParted(filepath.Base(os.Args[0]) + ".json")
	if err != nil {
		fatal("Failed to create parted instance: %v", err)
	}

	log("Loaded parted for disk " + p.Config.Disk)
	defer p.Close()

	// Print out information about the disk and its partitions.
	/*for i := 0; i < len(p.Partitions); i++ {
		partJSON, err := json.Marshal(p.Partitions[i], false)
		if err == nil {
			log(string(partJSON))
		}
	}*/

	log("Disk model: %s", p.DiskModel)
	log("Disk total size: %s (%s logical / %s physical)", bytes(p.DiskSize), bytes(p.SectorSizeLogical), bytes(p.SectorSizePhysical))
	log("Disk flags: %s", p.DiskFlags)
	log("Partition table: %s", p.PartitionTable)
	log("Size of partition table: %s (partitions: %s)", bytes(p.TableSize), bytes(p.PartsSize))

	// Calculate the total amount of space that needs to be reserved for the new partition table.
	// Store the reserved partitions and the actual partitions that will be modified.
	reserve := int64(0)
	partsReserved := make([]*Partition, 0)
	partsActual := make([]*Partition, 0)
	partsCreate := make([]*Partition, 0)
	for i := 0; i < len(p.Config.Reserved); i++ {
		// Add the expected reservation to the total reserve size.
		partReserved := p.Config.Reserved[i]
		reservedSize := partReserved.GetSize()
		reserve += reservedSize

		// Check if the reserved partition matches an existing partition on the disk.
		partActual := p.GetPartition(false, partReserved)
		if partActual == nil {
			log("Reserved partition %s could not be matched to disk, adding to create list", *partReserved.Name)
			partsCreate = append(partsCreate, partReserved)
		}

		// Remove the actual size of the existing partition from the total reserve count.
		// If reserve is 300MiB and actual is 400MiB, subtracting 400MiB results in -100MiB.
		actualSize := partActual.GetSize()
		reserve -= actualSize // Subtract only the amount that's already reserved.

		//Add the reserved partition to the reserved partitions list
		partsReserved = append(partsReserved, partReserved)
		//Add the actual partition to the actual partitions list
		partsActual = append(partsActual, partActual)
	}

	if len(partsReserved) == 0 || len(partsActual) == 0 {
		fatal("No reserved partitions specified for resizing")
	}

	// Calculate space to be freed or reserved for new partition table.
	// A positive reserve size is the size that will be taken from userdata.
	// A negative reserve size is the size that will be awarded to userdata.
	if reserve > 0 {
		log("Need to reserve %s from userdata for new partition table", bytes(reserve))
	} else if reserve < 0 {
		log("Need to award %s to userdata for new partition table", bytes(reserve * -1))
	} else {
		log("No additional space will be freed or reserved for new partition table")
	}

	partsReservedUserData := p.GetUserDataPartitions(true)
	if len(partsReservedUserData) == 0 {
		fatal("No userdata partitions specified for resizing")
	}

	partsActualUserData := p.GetUserDataPartitions(false)
	if len(partsActualUserData) != len(partsReservedUserData) {
		fatal("Actual count of userdata partitions (%d) does not match count in config (%d), too risky", len(partsActualUserData), len(partsReservedUserData))
	}

	sizeUserData := int64(0)
	for i := 0; i < len(partsActualUserData); i++ {
		sizeUserData += partsActualUserData[i].GetSize()
	}
	if reserve > sizeUserData {
		fatal("Need to reserve %s, %s larger than size of userdata %s", bytes(reserve), bytes(reserve - sizeUserData), bytes(sizeUserData))
	}

	partsShrink := make([]*Partition, 0)
	partsGrow := make([]*Partition, 0)
	partsMove := make([]*Partition, 0)
	for i := 0; i < len(partsReserved); i++ {
		if partsReserved[i].GetSize() < partsActual[i].GetSize() {
			log("Added to shrink list: %s (%s -> %s)", *partsReserved[i].Name, partsActual[i].GetSizeHuman(), partsReserved[i].GetSizeHuman())
			partsShrink = append(partsShrink, partsReserved[i])
		} else if partsReserved[i].GetSize() > partsActual[i].GetSize() {
			log("Added to grow list: %s (%s -> %s)", *partsReserved[i].Name, partsActual[i].GetSizeHuman(), partsReserved[i].GetSizeHuman())
			partsGrow = append(partsGrow, partsReserved[i])
		}
		if partsReserved[i].Number == nil {
			log("Added to move list: %s", *partsReserved[i].Name)
			partsMove = append(partsMove, partsReserved[i])
		}
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
	if len(msg) >= 1 {
		fatalMsg := "!!!FATAL!!! " + msg[0].(string)
		msg[0] = fatalMsg
	}
	log(msg...)
	os.Exit(1)
}
