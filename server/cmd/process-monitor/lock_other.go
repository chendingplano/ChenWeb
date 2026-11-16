//go:build !darwin && !linux

package main

import "fmt"

func lock(path string) (func(), error) {
	return nil, fmt.Errorf("process monitor requires Linux systemd")
}
