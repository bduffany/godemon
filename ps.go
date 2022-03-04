package main

import (
	"github.com/mitchellh/go-ps"
)

// descendantPids returns the pids of all currently running child processes
// (direct and indirect) of the process with the given pid.
func descendantPids(pid int) ([]int, error) {
	processes, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	ppid := make(map[int]int, len(processes))
	for _, p := range processes {
		ppid[p.Pid()] = p.PPid()
	}
	var childPids []int
	for _, p := range processes {
		cur := p.Pid()
		for {
			ppid, ok := ppid[cur]
			if !ok {
				break
			}
			cur = ppid
			if cur == pid {
				childPids = append(childPids, p.Pid())
				break
			}
		}
	}
	return childPids, nil
}

type ProcSnapshot struct {
	Processes map[int]ps.Process
}

func NewProcSnapshot() (*ProcSnapshot, error) {
	processes, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	snap := &ProcSnapshot{
		Processes: map[int]ps.Process{},
	}
	for _, p := range processes {
		snap.Processes[p.Pid()] = p
	}
	return snap, nil
}

// Diff returns a snapshot containing only processes which are different from
// the previous snapshot.
func (s *ProcSnapshot) Diff() (*ProcSnapshot, error) {
	n, err := NewProcSnapshot()
	if err != nil {
		return nil, err
	}
	diff := map[int]ps.Process{}
	for k, v := range n.Processes {
		if _, ok := s.Processes[k]; !ok {
			diff[k] = v
		}
	}
	return &ProcSnapshot{diff}, nil
}
