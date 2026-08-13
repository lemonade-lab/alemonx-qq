package main

import "os"

func startLuckyProcessDefault(platform *luckyPlatformSpec, root, entry string, log *os.File) (luckyProcess, error) {
	command, err := luckyStartCommand(platform, root, entry)
	if err != nil {
		return luckyProcess{}, err
	}
	command.Stdout, command.Stderr, command.Stdin = log, log, nil
	detachProcess(command)
	if err := command.Start(); err != nil {
		return luckyProcess{}, err
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	return luckyProcess{PID: pid, ProcessGroupID: pid}, nil
}
