package main

import (
	"os/exec"
	"strings"
)

func Run(prog, args string) (string, error) {
	cmd := strings.Split(prog, " ")
	cmdArgs := strings.Split(args, " ")
	if len(cmd) > 0 {
		cmdArgs = append(cmd[1:], cmdArgs...)
	}

	ret, err := exec.Command(cmd[0], cmdArgs...).CombinedOutput()
	if len(ret) > 0 || (err != nil && err.Error() == "signal: aborted") {
		err = nil //Hack to get around programs that exit non-zero or abort, we always want the output
	}
	//log("RUN: %s %v\n%s", cmd[0], cmdArgs, string(ret))
	//log("RUN: %s %v", cmd[0], cmdArgs)
	return string(ret), err
}