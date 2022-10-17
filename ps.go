package godemon

import (
	"os"
	"syscall"

	"github.com/mitchellh/go-ps"
)

// TreeSnapshot represents a snapshot of the process tree rooted at the current
// process tree.
type TreeSnapshot struct {
	Processes map[int]ps.Process
}

func NewTreeSnapshot() (*TreeSnapshot, error) {
	pid := os.Getpid()
	tree, err := Tree(pid)
	if err != nil {
		return nil, err
	}
	snap := &TreeSnapshot{Processes: tree}
	return snap, nil
}

// Diff returns a snapshot containing only processes which are different from
// the previous snapshot.
func (s *TreeSnapshot) Diff() (*TreeSnapshot, error) {
	n, err := NewTreeSnapshot()
	if err != nil {
		return nil, err
	}
	diff := map[int]ps.Process{}
	for k, v := range n.Processes {
		if _, ok := s.Processes[k]; !ok {
			diff[k] = v
		}
	}
	return &TreeSnapshot{diff}, nil
}

// KillProcessTree kills the given pid as well as any descendant processes.
//
// It tries to kill as many processes in the tree as possible. If it encounters
// an error along the way, it proceeds to kill subsequent pids in the tree. It
// returns the last error encountered, if any.
func KillProcessTree(pid int) error {
	var lastErr error

	// Run a BFS on the process tree to build up a list of processes to kill.
	// Before listing child processes for each pid, send SIGSTOP to prevent it
	// from spawning new child processes. Otherwise the child process list has a
	// chance to become stale if the pid forks a new child just after we list
	// processes but before we send SIGKILL.

	pidsToExplore := []int{pid}
	pidsToKill := []int{}
	for len(pidsToExplore) > 0 {
		pid := pidsToExplore[0]
		pidsToExplore = pidsToExplore[1:]
		if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
			lastErr = err
			// If we fail to SIGSTOP, proceed anyway; the more we can clean up,
			// the better.
		}
		pidsToKill = append(pidsToKill, pid)

		childPids, err := ChildPids(pid)
		if err != nil {
			lastErr = err
			continue
		}
		pidsToExplore = append(pidsToExplore, childPids...)
	}
	for _, pid := range pidsToKill {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// ChildPids returns all *direct* child pids of a process identified by pid.
func ChildPids(pid int) ([]int, error) {
	procs, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	return childPids(pid, procs), nil
}

func childPids(pid int, procs []ps.Process) []int {
	var out []int
	for _, proc := range procs {
		if proc.PPid() != pid {
			continue
		}
		out = append(out, proc.Pid())
	}
	return out
}

func childProcs(pid int, procs []ps.Process) []ps.Process {
	var out []ps.Process
	for _, proc := range procs {
		if proc.PPid() != pid {
			continue
		}
		out = append(out, proc)
	}
	return out
}

// TreePids returns all the pids in the tree rooted at the given pid, including
// the pid itself.
func TreePids(pid int) ([]int, error) {
	procs, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	f := []int{pid}
	var out []int
	for len(f) > 0 {
		pid, f = f[0], f[1:]
		out = append(out, pid)
		f = append(f, childPids(pid, procs)...)
	}
	return out, nil
}

func Tree(pid int) (map[int]ps.Process, error) {
	procs, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	var f []ps.Process
	var p ps.Process
	for _, proc := range procs {
		if proc.Pid() == pid {
			p = proc
			f = append(f, p)
			break
		}
	}
	out := map[int]ps.Process{}
	for len(f) > 0 {
		p, f = f[0], f[1:]
		out[p.Pid()] = p
		f = append(f, childProcs(p.Pid(), procs)...)
	}
	return out, nil
}
