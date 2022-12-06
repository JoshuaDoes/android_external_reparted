package main

import (
	"os/exec"
	"strings"
)

func Run(prog, args string) (string, error) {
	ret, err := exec.Command(prog, strings.Split(args, " ")...).Output()
	if len(ret) > 0 {
		err = nil //Hack to get around programs that exit non-zero, we always want the output
	}
	return string(ret), err
}