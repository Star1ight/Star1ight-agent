package main

import "syscall"

type syscallStatfs struct {
	blocks uint64
	bavail uint64
	bsize  int64
}

func statfs(path string, out *syscallStatfs) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return err
	}
	out.blocks = uint64(s.Blocks)
	out.bavail = uint64(s.Bavail)
	out.bsize = int64(s.Bsize)
	return nil
}
