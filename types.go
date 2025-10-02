package nmea

import (
	"sync"
	"time"
)

// RawFrame represents an unparsed NMEA2000 CAN frame
type RawFrame struct {
	Timestamp time.Time
	Topic     string
	ID        uint32
	Priority  uint8
	DP        uint8
	PF        uint8
	PS        uint8
	Source    uint8
	Dest      uint8
	PGN       int
	Length    int
	Data      []byte
}

// DecodedMessage represents a fully decoded NMEA2000 message
type DecodedMessage struct {
	Timestamp   time.Time
	PGN         int
	PGNName     string
	Source      uint8
	Measurement string
	Fields      map[string]interface{}
	Raw         []byte
}

// Statistics tracks collector performance metrics
type Statistics struct {
	mu                sync.RWMutex
	MessagesProcessed int64
	DecodeSuccesses   int64
	DecodeFailures    int64
	PGNCounts         map[int]int64
	MeasurementCounts map[string]int64
	LastUpdate        time.Time
	StartTime         time.Time
}

func NewStatistics() *Statistics {
	return &Statistics{
		PGNCounts:         make(map[int]int64),
		MeasurementCounts: make(map[string]int64),
		StartTime:         time.Now(),
		LastUpdate:        time.Now(),
	}
}

func (s *Statistics) RecordMessage(pgn int, measurement string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.MessagesProcessed++
	if success {
		s.DecodeSuccesses++
	} else {
		s.DecodeFailures++
	}

	s.PGNCounts[pgn]++
	s.MeasurementCounts[measurement]++
	s.LastUpdate = time.Now()
}

func (s *Statistics) GetSnapshot() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	successRate := 0.0
	if s.MessagesProcessed > 0 {
		successRate = float64(s.DecodeSuccesses) / float64(s.MessagesProcessed) * 100.0
	}

	uptime := time.Since(s.StartTime)
	msgPerSec := 0.0
	if uptime.Seconds() > 0 {
		msgPerSec = float64(s.MessagesProcessed) / uptime.Seconds()
	}

	return map[string]interface{}{
		"messages_processed": s.MessagesProcessed,
		"decode_successes":   s.DecodeSuccesses,
		"decode_failures":    s.DecodeFailures,
		"success_rate":       successRate,
		"uptime_seconds":     uptime.Seconds(),
		"messages_per_sec":   msgPerSec,
		"last_update":        s.LastUpdate,
	}
}

// Config holds NMEA collector configuration
type Config struct {
	MQTTBroker       string
	MQTTPort         int
	MQTTUsername     string
	MQTTPassword     string
	MQTTTopic        string
	UseTLS           bool
	InsecureSkipTLS  bool
	DeviceID         string
	BufferSize       int
	DecoderWorkers   int
	QueueSize        int
	EnableCSV        bool
	CSVFramesPath    string
	CSVDecodedPath   string
	CSVStatsPath     string
}

func DefaultConfig() Config {
	return Config{
		MQTTBroker:      "02c55b5f93704f9eb9883f5c7bc98e8c.s1.eu.hivemq.cloud",
		MQTTPort:        8883,
		MQTTUsername:    "esp32",
		MQTTPassword:    "Pourquoi312",
		MQTTTopic:       "boats/esp32s3-dev01/#",
		UseTLS:          true,
		InsecureSkipTLS: false,
		DeviceID:        "esp32s3-dev01",
		BufferSize:      86400,
		DecoderWorkers:  4,
		QueueSize:       1000,
		EnableCSV:       true,
		CSVFramesPath:   "data/frames.csv",
		CSVDecodedPath:  "data/decoded_long.csv",
		CSVStatsPath:    "data/decode_stats.csv",
	}
}