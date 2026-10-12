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
	// proc stays open until release: Windows doesn't reuse the pid of a
	// process someone holds a handle to, so the sweep can't hit a stranger
	proc windows.Handle
}

func guardProcess(p *os.Process) procGuard {
	var g procGuard
	if p == nil {
		return g
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return g
	}
	g.proc = h

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
	if g.proc != 0 {
		for _, h := range descendants(g.proc) {
			windows.TerminateProcess(h, 1)
			windows.CloseHandle(h)
		}
		windows.CloseHandle(g.proc)
	}
	if g.job != 0 {
		windows.CloseHandle(g.job)
	}
}

// descendants opens the processes started by root, and by those, and so on.
// Windows keeps a process's parent id after the parent exits, so this works
// once the browser is gone too. A pid can be reused once its process has
// exited, so a child only counts if it was created after its parent.
func descendants(root windows.Handle) []windows.Handle {
	rootPid, err := windows.GetProcessId(root)
	if err != nil {
		return nil
	}
	rootCreated, ok := created(root)
	if !ok {
		return nil
	}

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

	type node struct {
		pid     uint32
		created int64
	}
	var out []windows.Handle
	seen := map[uint32]bool{rootPid: true}
	queue := []node{{rootPid, rootCreated}}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, pid := range children[parent.pid] {
			if seen[pid] {
				continue
			}
			seen[pid] = true
			// the handle is kept open (in out) so this pid can't be reused
			// before it's terminated
			h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err != nil {
				continue
			}
			c, ok := created(h)
			if !ok || c < parent.created {
				windows.CloseHandle(h)
				continue
			}
			out = append(out, h)
			queue = append(queue, node{pid, c})
		}
	}
	return out
}

// created returns when a process was created, in Windows file time
func created(h windows.Handle) (int64, bool) {
	var c, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &exit, &kernel, &user); err != nil {
		return 0, false
	}
	return c.Nanoseconds(), true
}
