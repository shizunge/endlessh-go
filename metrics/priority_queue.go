package metrics

import (
	"container/heap"
	"time"
)

// Pair represents a key-value pair with a timestamp
type Pair struct {
	Key     string
	Value   time.Time
	HeapIdx int // Index in the heap for efficient updates
}

// PriorityQueue is a min-heap implementation for Pairs
type PriorityQueue []*Pair

// Len returns the length of the priority queue
func (pq PriorityQueue) Len() int { return len(pq) }

// Less compares two pairs based on their values (timestamps)
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Value.Before(pq[j].Value)
}

// Swap swaps two pairs in the priority queue
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].HeapIdx = i
	pq[j].HeapIdx = j
}

// Push adds a pair to the priority queue
func (pq *PriorityQueue) Push(x interface{}) {
	pair := x.(*Pair)
	pair.HeapIdx = len(*pq)
	*pq = append(*pq, pair)
}

// Pop removes the pair with the minimum value (timestamp) from the priority queue
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	pair := old[n-1]
	pair.HeapIdx = -1 // for safety
	*pq = old[0 : n-1]
	return pair
}

// UpdatablePriorityQueue represents the data structure with the priority queue
type UpdatablePriorityQueue struct {
	pq     PriorityQueue
	keyMap map[string]*Pair
}

// NewUpdatablePriorityQueue initializes a new UpdatablePriorityQueue
func NewUpdatablePriorityQueue() *UpdatablePriorityQueue {
	return &UpdatablePriorityQueue{
		pq:     make(PriorityQueue, 0),
		keyMap: make(map[string]*Pair),
	}
}

// Update adds or updates a key-value pair in the data structure
func (ds *UpdatablePriorityQueue) Update(key string, value time.Time) {
	if pair, ok := ds.keyMap[key]; ok {
		// Key exists, update the time
		pair.Value = value
		heap.Fix(&ds.pq, pair.HeapIdx)
	} else {
		// Key does not exist, create a new entry
		pair := &Pair{Key: key, Value: value}
		heap.Push(&ds.pq, pair)
		ds.keyMap[key] = pair
	}
}

// Peek returns the entry with the minimal time
func (ds *UpdatablePriorityQueue) Peek() *Pair {
	if ds.pq.Len() == 0 {
		return nil
	}
	return ds.pq[0]
}

// Pop removes the entry with the minimal time
func (ds *UpdatablePriorityQueue) Pop() *Pair {
	if ds.pq.Len() == 0 {
		return nil
	}
	pair := heap.Pop(&ds.pq).(*Pair)
	delete(ds.keyMap, pair.Key)
	return pair
}
