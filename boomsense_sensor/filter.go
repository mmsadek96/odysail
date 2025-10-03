package boomsense_sensor

import (
	"math"
	"sync"
)

// ComplementaryFilter implements Euler angle estimation from IMU
type ComplementaryFilter struct {
	tau          float64
	initialized  bool
	roll         float64
	pitch        float64
	lastTime     float64
	mu           sync.RWMutex
}

func NewComplementaryFilter(tau float64) *ComplementaryFilter {
	return &ComplementaryFilter{
		tau: tau,
	}
}

// Update processes new IMU reading and returns filtered roll and pitch
func (cf *ComplementaryFilter) Update(reading IMUReading) (roll, pitch float64) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	ts := float64(reading.Timestamp.UnixNano()) / 1e9

	// Remap coordinates to stern-view frame:
	// Desired frame: +X=starboard, +Y=up, +Z=forward (bow)
	// Python mapping: ax, ay, az = ay, -az, ax
	//                 gx, gy, gz = gy, -gz, gx
	ax := reading.AccelY
	ay := -reading.AccelZ
	az := reading.AccelX
	gx := reading.GyroY
	gy := -reading.GyroZ

	if !cf.initialized {
		// Initialize from accelerometer
		rollAcc, pitchAcc := cf.accTiltDeg(ax, ay, az)
		cf.roll = rollAcc
		cf.pitch = pitchAcc
		cf.lastTime = ts
		cf.initialized = true
		return cf.roll, cf.pitch
	}

	// Calculate time delta
	dt := ts - cf.lastTime
	if dt > 0.2 {
		dt = 0.2 // Cap large gaps
	}
	cf.lastTime = ts

	// Integrate gyroscope (prediction step)
	rollGyro := cf.roll + gx*dt
	pitchGyro := cf.pitch + gy*dt

	// Get accelerometer angles (measurement step)
	rollAcc, pitchAcc := cf.accTiltDeg(ax, ay, az)

	// Complementary filter fusion
	tau := math.Max(1e-3, cf.tau)
	alpha := tau / (tau + dt)
	if dt <= 0 {
		alpha = 1.0
	}

	cf.roll = alpha*rollGyro + (1.0-alpha)*rollAcc
	cf.pitch = alpha*pitchGyro + (1.0-alpha)*pitchAcc

	return cf.roll, cf.pitch
}

// accTiltDeg calculates roll and pitch from accelerometer in stern-view frame
// Stern-view frame: +X=starboard, +Y=up, +Z=forward (bow)
// Gravity at rest ≈ (0, -1g, 0)
// Roll (about +X, starboard axis): φ = atan2(az, -ay)
// Pitch (about +Y, up axis): θ = atan2(-ax, sqrt(ay² + az²))
func (cf *ComplementaryFilter) accTiltDeg(ax, ay, az float64) (roll, pitch float64) {
	roll = math.Atan2(az, -ay) * 180.0 / math.Pi
	pitch = math.Atan2(-ax, math.Sqrt(ay*ay+az*az)) * 180.0 / math.Pi
	return
}

// GetState returns current filtered angles (thread-safe)
func (cf *ComplementaryFilter) GetState() (roll, pitch float64, initialized bool) {
	cf.mu.RLock()
	defer cf.mu.RUnlock()
	return cf.roll, cf.pitch, cf.initialized
}

// Reset clears the filter state
func (cf *ComplementaryFilter) Reset() {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	cf.initialized = false
	cf.roll = 0.0
	cf.pitch = 0.0
	cf.lastTime = 0.0
}