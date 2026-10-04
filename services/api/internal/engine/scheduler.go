package engine

import "container/heap"

// rank orders work that falls on the same simulation instant. Things that
// end are processed before things that begin, and the communication state is
// settled before any information moves, so a report generated at the instant
// a dropout starts is subject to it, and one generated at the instant it ends
// is not.
type rank int

const (
	rankPhaseCompleted rank = iota
	rankPhaseStarted
	rankRuleEnd
	rankRuleStart
	rankDecisionClose
	rankRelease // a delayed item reaching its recipients
	rankGenerate
	rankDecisionOpen
	rankExerciseEnd
)

type task struct {
	at   SimMs
	rank rank
	seq  uint64 // insertion order: the final tie-break
	// index into the relevant definition slice, or the pending delivery id
	index    int
	delivery *delivery
}

// Scheduler is a priority queue of future work with a total order:
// time, then rank, then insertion order. Nothing about its output depends on
// map iteration, goroutine timing or how often it is polled.
type Scheduler struct {
	h    taskHeap
	next uint64
}

func (s *Scheduler) Schedule(t task) {
	t.seq = s.next
	s.next++
	heap.Push(&s.h, t)
}

// PopDue removes and returns the next task due at or before now.
func (s *Scheduler) PopDue(now SimMs) (task, bool) {
	if len(s.h) == 0 || s.h[0].at > now {
		return task{}, false
	}
	return heap.Pop(&s.h).(task), true
}

func (s *Scheduler) Len() int { return len(s.h) }

func (s *Scheduler) Clear() { s.h = nil }

type taskHeap []task

func (h taskHeap) Len() int { return len(h) }
func (h taskHeap) Less(i, j int) bool {
	a, b := h[i], h[j]
	if a.at != b.at {
		return a.at < b.at
	}
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	return a.seq < b.seq
}
func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *taskHeap) Push(x any)   { *h = append(*h, x.(task)) }
func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	*h = old[:n-1]
	return t
}
