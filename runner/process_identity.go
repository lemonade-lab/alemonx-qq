package main

// napcatProcess identifies the actual QQ runtime separately from the process
// group that owns every helper created for it. On Linux the group leader is
// the Xvfb server while PID is the QQ/NapCat runtime.
type napcatProcess struct {
	PID            int
	ProcessGroupID int
}
