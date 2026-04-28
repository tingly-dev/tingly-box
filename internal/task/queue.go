package task

import "sync"

// serialKeyQueue maintains an in-memory FIFO of queued task IDs per
// serialization key. It is rebuilt from the DB after every restart.
type serialKeyQueue struct {
	mu    sync.Mutex
	queue map[string][]string // serializationKey → ordered taskIDs
}

func newSerialKeyQueue() *serialKeyQueue {
	return &serialKeyQueue{queue: make(map[string][]string)}
}

// enqueue appends taskID to the FIFO for key.
func (q *serialKeyQueue) enqueue(key, taskID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue[key] = append(q.queue[key], taskID)
}

// dequeue removes and returns the front taskID for key, or "" if empty.
func (q *serialKeyQueue) dequeue(key string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue[key]) == 0 {
		return ""
	}
	id := q.queue[key][0]
	q.queue[key] = q.queue[key][1:]
	if len(q.queue[key]) == 0 {
		delete(q.queue, key)
	}
	return id
}

// remove deletes taskID from any position in key's queue (used on cancel).
func (q *serialKeyQueue) remove(key, taskID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := q.queue[key]
	for i, id := range list {
		if id == taskID {
			q.queue[key] = append(list[:i], list[i+1:]...)
			if len(q.queue[key]) == 0 {
				delete(q.queue, key)
			}
			return
		}
	}
}

// peek returns the front taskID for key without removing it, or "".
func (q *serialKeyQueue) peek(key string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue[key]) == 0 {
		return ""
	}
	return q.queue[key][0]
}

// depth returns the queue depth for key.
func (q *serialKeyQueue) depth(key string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue[key])
}
