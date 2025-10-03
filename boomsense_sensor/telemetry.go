package boomsense_sensor

import (
	"sync"
	"time"
)

// TelemetryBuffers holds ring buffers for all sensor data
type TelemetryBuffers struct {
	imu   *RingBuffer
	meteo *RingBuffer
	wind  *RingBuffer
	mu    sync.RWMutex
}

func NewTelemetryBuffers(maxLen int) *TelemetryBuffers {
	return &TelemetryBuffers{
		imu:   NewRingBuffer(maxLen),
		meteo: NewRingBuffer(maxLen),
		wind:  NewRingBuffer(maxLen),
	}
}

// PushIMU adds IMU reading to buffer
func (tb *TelemetryBuffers) PushIMU(reading IMUReading) {
	tb.imu.Push(reading)
}

// PushMeteo adds meteo reading to buffer
func (tb *TelemetryBuffers) PushMeteo(reading MeteoReading) {
	tb.meteo.Push(reading)
}

// PushWind adds wind reading to buffer
func (tb *TelemetryBuffers) PushWind(reading WindReading) {
	tb.wind.Push(reading)
}

// PushFiltered adds filtered data to buffer (for visualization/logging)
func (tb *TelemetryBuffers) PushFiltered(data FilteredData) {
	// Store in IMU buffer as enhanced reading
	tb.imu.Push(data)
}

// GetLatestWind returns most recent wind data
func (tb *TelemetryBuffers) GetLatestWind() (WindReading, bool) {
	recent := tb.wind.GetRecent(1)
	if len(recent) == 0 {
		return WindReading{}, false
	}
	if w, ok := recent[0].(WindReading); ok {
		return w, true
	}
	return WindReading{}, false
}

// GetRecentIMU returns last n IMU readings
func (tb *TelemetryBuffers) GetRecentIMU(n int) []IMUReading {
	items := tb.imu.GetRecent(n)
	result := make([]IMUReading, 0, len(items))
	for _, item := range items {
		if imu, ok := item.(IMUReading); ok {
			result = append(result, imu)
		}
	}
	return result
}

// GetRecentFiltered returns last n filtered readings
func (tb *TelemetryBuffers) GetRecentFiltered(n int) []FilteredData {
	items := tb.imu.GetRecent(n)
	result := make([]FilteredData, 0, len(items))
	for _, item := range items {
		if filt, ok := item.(FilteredData); ok {
			result = append(result, filt)
		}
	}
	return result
}

// GetRecentMeteo returns last n meteo readings
func (tb *TelemetryBuffers) GetRecentMeteo(n int) []MeteoReading {
	items := tb.meteo.GetRecent(n)
	result := make([]MeteoReading, 0, len(items))
	for _, item := range items {
		if m, ok := item.(MeteoReading); ok {
			result = append(result, m)
		}
	}
	return result
}

// GetRecentWind returns last n wind readings
func (tb *TelemetryBuffers) GetRecentWind(n int) []WindReading {
	items := tb.wind.GetRecent(n)
	result := make([]WindReading, 0, len(items))
	for _, item := range items {
		if w, ok := item.(WindReading); ok {
			result = append(result, w)
		}
	}
	return result
}

// GetTimeRange returns all data within time window
func (tb *TelemetryBuffers) GetTimeRange(start, end time.Time) (imu []IMUReading, meteo []MeteoReading, wind []WindReading) {
	// IMU
	allIMU := tb.imu.GetAll()
	for _, item := range allIMU {
		if r, ok := item.(IMUReading); ok {
			if (r.Timestamp.Equal(start) || r.Timestamp.After(start)) &&
				(r.Timestamp.Equal(end) || r.Timestamp.Before(end)) {
				imu = append(imu, r)
			}
		}
	}

	// Meteo
	allMeteo := tb.meteo.GetAll()
	for _, item := range allMeteo {
		if r, ok := item.(MeteoReading); ok {
			if (r.Timestamp.Equal(start) || r.Timestamp.After(start)) &&
				(r.Timestamp.Equal(end) || r.Timestamp.Before(end)) {
				meteo = append(meteo, r)
			}
		}
	}

	// Wind
	allWind := tb.wind.GetAll()
	for _, item := range allWind {
		if r, ok := item.(WindReading); ok {
			if (r.Timestamp.Equal(start) || r.Timestamp.After(start)) &&
				(r.Timestamp.Equal(end) || r.Timestamp.Before(end)) {
				wind = append(wind, r)
			}
		}
	}

	return
}

// Stats returns buffer statistics
func (tb *TelemetryBuffers) Stats() map[string]interface{} {
	return map[string]interface{}{
		"imu_size":   tb.imu.Size(),
		"meteo_size": tb.meteo.Size(),
		"wind_size":  tb.wind.Size(),
	}
}