package boomsense_sensor

import (
	"sync"
	"time"
)

// IMUReading represents raw IMU sensor data
type IMUReading struct {
	Timestamp time.Time
	AccelX    float64 // g
	AccelY    float64 // g
	AccelZ    float64 // g
	GyroX     float64 // deg/s
	GyroY     float64 // deg/s
	GyroZ     float64 // deg/s
}

// MeteoReading represents meteorological sensor data
type MeteoReading struct {
	Timestamp   time.Time
	TempC       float64
	PressureHpa float64
	HumidityPct float64
}

// WindReading represents wind sensor data
type WindReading struct {
	Timestamp time.Time
	SpeedKts  float64
	AngleDeg  float64
}

// FilteredData represents processed IMU data with filtered angles
type FilteredData struct {
	Timestamp    time.Time
	RollDeg      float64
	PitchDeg     float64
	BoomRelDeg   float64 // Relative to calibrated center
	BoomNorm     float64 // Normalized [-1, 1]
	AccelX       float64
	AccelY       float64
	AccelZ       float64
	GyroX        float64
	GyroY        float64
	GyroZ        float64
}

// Calibration holds boom calibration parameters
type Calibration struct {
	Mid      float64 // Center angle (degrees)
	SpanPos  float64 // Starboard span (degrees)
	SpanNeg  float64 // Port span (degrees)
	Timestamp time.Time
}

// Event represents a detected sailing event
type Event struct {
	Type      string    // "tack", "gybe_normal", "gybe_crash", "boom_hit"
	Timestamp time.Time
	GyroPeak  float64
	BoomDelta float64
	RollDelta float64
	Duration  float64
	Direction string  // For tacks: "stb_to_port", "port_to_stb"
	Overshoot float64 // For tacks
	Score     float64 // Tack quality score (0-100)
	WindSpeed float64
	WindAngle float64
}

// RingBuffer is a generic circular buffer
type RingBuffer struct {
	data     []interface{}
	head     int
	size     int
	capacity int
	mu       sync.RWMutex
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]interface{}, capacity),
		capacity: capacity,
	}
}

func (rb *RingBuffer) Push(item interface{}) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.head] = item
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
}

func (rb *RingBuffer) GetRecent(n int) []interface{} {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.size {
		n = rb.size
	}

	result := make([]interface{}, n)
	for i := 0; i < n; i++ {
		idx := (rb.head - 1 - i + rb.capacity) % rb.capacity
		result[i] = rb.data[idx]
	}
	return result
}

func (rb *RingBuffer) Size() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size
}

func (rb *RingBuffer) GetAll() []interface{} {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]interface{}, rb.size)
	for i := 0; i < rb.size; i++ {
		idx := (rb.head - rb.size + i + rb.capacity) % rb.capacity
		result[i] = rb.data[idx]
	}
	return result
}

// Config holds sensor configuration
type Config struct {
	MaxBufferSize int
	EulerTau      float64
	BoomAxis      string // "roll" or "pitch"
	
	// Event detection thresholds
	CrashGyDPS        float64
	NormalGyMin       float64
	BoomStepCrash     float64
	BoomStepNormal    float64
	CrashDT           float64
	NormalDT          float64
	RollHit           float64
	RollDT            float64
	TackGyMin         float64
	TackGyMax         float64
	TackBoomStep      float64
	TackDTMax         float64
	TackMinRollDelta  float64
	
	// Bayesian QA
	BayesSigma0       float64
	QALowThreshold    float64
	QAHighThreshold   float64
	
	RefractoryPeriod  float64 // seconds between events
}

func DefaultConfig() Config {
	return Config{
		MaxBufferSize:    600,
		EulerTau:         0.7,
		BoomAxis:         "roll",
		CrashGyDPS:       120.0,
		NormalGyMin:      20.0,
		BoomStepCrash:    1.2,
		BoomStepNormal:   1.0,
		CrashDT:          0.6,
		NormalDT:         2.5,
		RollHit:          8.0,
		RollDT:           0.4,
		TackGyMin:        15.0,
		TackGyMax:        110.0,
		TackBoomStep:     1.0,
		TackDTMax:        3.0,
		TackMinRollDelta: 12.0,
		BayesSigma0:      10.0,
		QALowThreshold:   0.02,
		QAHighThreshold:  0.85,
		RefractoryPeriod: 3.0,
	}
}