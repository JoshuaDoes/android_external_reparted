package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func Run(prog, args string) (string, error) {
	ret, err := exec.Command(prog, strings.Split(args, " ")...).Output()
	if err != nil {
		return "", fmt.Errorf("Failed to call %s: %v", prog, err)
	}

	return string(ret), nil
}