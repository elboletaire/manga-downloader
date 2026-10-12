// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build windows

package browser

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procGuard keeps a browser's processes from outliving the app on Windows,
// where killing the browser process (what chromedp does) leaves its children
// running, and nothing at all happens to it when the app exits without
// closing it (killed, crashed, or a shutdown that timed out).
//
// The browser goes into a job object flagged kill-on-close: every process it
// starts from then on joins the job, and Windows kills them all when the job
// handle closes, which release does, and which the OS does itself when this
// process ends, however it ends.
type procGuard struct {
	job windows.Handle
	pid uint32
}

func guardProcess(p *os.Process) procGuard {
	if p == nil {
		return procGuard{}
	}
	g := procGuard{pid: uint32(p.Pid)}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return g
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return g
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, g.pid)
	if err != nil {
		windows.CloseHandle(job)
		return g
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return g
	}
	g.job = job
	return g
}

// release is called once the browser process has been stopped. It kills the
// helper processes the browser started before it was put in the job (they
// were already running by the time chromedp handed it over), then closes the
// job, which kills everything left in it.
func (g procGuard) release() {
	if g.pid != 0 {
		for _, pid := range descendants(g.pid) {
			if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
				windows.TerminateProcess(h, 1)
				windows.CloseHandle(h)
			}
		}
	}
	if g.job != 0 {
		windows.CloseHandle(g.job)
	}
}

// descendants lists the processes started by pid, and by those, and so on.
// Windows keeps a process's parent id after the parent exits, so this still
// works once the browser process itself is gone.
func descendants(pid uint32) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)

	children := map[uint32][]uint32{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID != e.ParentProcessID {
			children[e.ParentProcessID] = append(children[e.ParentProcessID], e.ProcessID)
		}
	}

	var out []uint32
	seen := map[uint32]bool{pid: true}
	queue := []uint32{pid}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range children[p] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
				queue = append(queue, c)
			}
		}
	}
	return out
}
