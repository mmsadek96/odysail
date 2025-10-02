package storage

import (
	"sync"
	"time"
)

// DecodedMessage local definition (already in csv_writer.go, shared in package)

type RingBuffer struct {
	data     []DecodedMessage
	head     int
	size     int
	capacity int
	mu       sync.RWMutex

	latestByPGN map[int]*DecodedMessage
	indexMu     sync.RWMutex
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:        make([]DecodedMessage, capacity),
		capacity:    capacity,
		latestByPGN: make(map[int]*DecodedMessage),
	}
}

func (rb *RingBuffer) Push(msg DecodedMessage) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.head] = msg
	rb.head = (rb.head + 1) % rb.capacity

	if rb.size < rb.capacity {
		rb.size++
	}

	rb.indexMu.Lock()
	rb.latestByPGN[msg.PGN] = &msg
	rb.indexMu.Unlock()
}

func (rb *RingBuffer) GetRecent(n int) []DecodedMessage {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.size {
		n = rb.size
	}

	result := make([]DecodedMessage, n)
	for i := 0; i < n; i++ {
		idx := (rb.head - 1 - i + rb.capacity) % rb.capacity
		result[i] = rb.data[idx]
	}

	return result
}

func (rb *RingBuffer) GetByTimeRange(start, end time.Time) []DecodedMessage {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]DecodedMessage, 0)

	for i := 0; i < rb.size; i++ {
		msg := rb.data[i]
		if (msg.Timestamp.Equal(start) || msg.Timestamp.After(start)) &&
			(msg.Timestamp.Equal(end) || msg.Timestamp.Before(end)) {
			result = append(result, msg)
		}
	}

	return result
}

func (rb *RingBuffer) GetLatestByPGN(pgn int) *DecodedMessage {
	rb.indexMu.RLock()
	defer rb.indexMu.RUnlock()

	if msg, ok := rb.latestByPGN[pgn]; ok {
		return msg
	}
	return nil
}

func (rb *RingBuffer) Size() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size
}

func (rb *RingBuffer) Capacity() int {
	return rb.capacity
}

func (rb *RingBuffer) GetStats() map[string]interface{} {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	oldest := time.Time{}
	newest := time.Time{}

	if rb.size > 0 {
		oldestIdx := (rb.head - rb.size + rb.capacity) % rb.capacity
		oldest = rb.data[oldestIdx].Timestamp

		newestIdx := (rb.head - 1 + rb.capacity) % rb.capacity
		newest = rb.data[newestIdx].Timestamp
	}

	return map[string]interface{}{
		"size":              rb.size,
		"capacity":          rb.capacity,
		"utilization":       float64(rb.size) / float64(rb.capacity) * 100.0,
		"oldest_timestamp":  oldest,
		"newest_timestamp":  newest,
		"time_span_seconds": newest.Sub(oldest).Seconds(),
	}
}