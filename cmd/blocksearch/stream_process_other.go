//go:build !unix

package main

import "os/exec"

func configureStreamProcess(cmd *exec.Cmd) {}
