package boomsense_sensor

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"
)

// Sensor is the main BoomSense coordinator
type Sensor struct {
	config     Config
	filter     *ComplementaryFilter
	calibrator *BoomCalibrator
	detector   *EventDetector
	bayesian   *BayesianQA
	buffers    *TelemetryBuffers
	csvWriter  *csv.Writer
	csvFile    *os.File
	startTime  time.Time
	mu         sync.RWMutex
}

// NewSensor creates a new BoomSense sensor
func NewSensor(config Config) *Sensor {
	s := &Sensor{
		config:     config,
		filter:     NewComplementaryFilter(config.EulerTau),
		calibrator: NewBoomCalibrator(config.BoomAxis),
		detector:   NewEventDetector(config),
		bayesian:   NewBayesianQA(11, config.BayesSigma0), // 11 features with wind
		buffers:    NewTelemetryBuffers(config.MaxBufferSize),
		startTime:  time.Now(),
	}

	return s
}

// Start initializes the sensor
func (s *Sensor) Start() error {
	log.Printf("[BoomSense] Starting sensor...")
	log.Printf("[BoomSense] Config: EulerTau=%.2f BoomAxis=%s", s.config.EulerTau, s.config.BoomAxis)

	// Try to load existing calibration
	if err := s.calibrator.LoadFromFile("boom_calibration.json"); err == nil {
		cal := s.calibrator.GetCalibration()
		if cal != nil {
			log.Printf("[BoomSense] Loaded calibration: mid=%.2f span_pos=%.2f span_neg=%.2f", 
				cal.Mid, cal.SpanPos, cal.SpanNeg)
		}
	}

	// Try to load Bayesian model
	if err := s.bayesian.LoadState("boom_bayes_posterior.json"); err == nil {
		log.Printf("[BoomSense] Loaded Bayesian QA model")
	}

	log.Printf("[BoomSense] Sensor started successfully")
	return nil
}

// Stop gracefully shuts down the sensor
func (s *Sensor) Stop() {
	log.Printf("[BoomSense] Stopping sensor...")

	// Save calibration
	if err := s.calibrator.SaveToFile("boom_calibration.json"); err == nil {
		log.Printf("[BoomSense] Saved calibration")
	}

	// Save Bayesian model
	if err := s.bayesian.SaveState("boom_bayes_posterior.json"); err == nil {
		log.Printf("[BoomSense] Saved Bayesian QA model")
	}

	// Close CSV
	if s.csvWriter != nil {
		s.csvWriter.Flush()
		s.csvFile.Close()
	}

	log.Printf("[BoomSense] Sensor stopped")
}

// EnableCSVLogging starts CSV output
func (s *Sensor) EnableCSVLogging(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	s.csvFile = file
	s.csvWriter = csv.NewWriter(file)

	// Write header if file is new
	info, _ := file.Stat()
	if info.Size() == 0 {
		header := []string{
			"iso8601", "ts", "ax_g", "ay_g", "az_g",
			"gx_dps", "gy_dps", "gz_dps",
			"roll_f_deg", "pitch_f_deg",
			"boom_rel_deg", "boom_norm",
			"temp_c", "press_hpa", "rh_pct",
			"wind_speed_kn", "wind_angle_deg",
		}
		s.csvWriter.Write(header)
		s.csvWriter.Flush()
	}

	return nil
}

// ProcessIMU processes an IMU reading
func (s *Sensor) ProcessIMU(reading IMUReading) FilteredData {
	// Apply complementary filter
	roll, pitch := s.filter.Update(reading)

	// Get axis value based on config
	axisValue := roll
	if s.config.BoomAxis == "pitch" {
		axisValue = pitch
	}

	// Compute boom metrics
	boomRelDeg, boomNorm, hasCal := s.calibrator.ComputeBoom(axisValue)

	// Build filtered data
	filtered := FilteredData{
		Timestamp:  reading.Timestamp,
		RollDeg:    roll,
		PitchDeg:   pitch,
		BoomRelDeg: boomRelDeg,
		BoomNorm:   boomNorm,
		AccelX:     reading.AccelX,
		AccelY:     reading.AccelY,
		AccelZ:     reading.AccelZ,
		GyroX:      reading.GyroX,
		GyroY:      reading.GyroY,
		GyroZ:      reading.GyroZ,
	}

	// Handle NaN for uncalibrated
	if !hasCal {
		filtered.BoomRelDeg = math.NaN()
		filtered.BoomNorm = math.NaN()
	}

	// Store in buffer
	s.buffers.PushFiltered(filtered)

	// Feed to event detector
	if hasCal && !math.IsNaN(filtered.BoomNorm) && !math.IsInf(filtered.BoomNorm, 0) {
		s.detector.OnSample(reading.Timestamp, reading.GyroY, filtered.BoomNorm, roll)
	}

	// Write to CSV
	s.writeCSVRow(filtered)

	return filtered
}

// ProcessMeteo processes a meteo reading
func (s *Sensor) ProcessMeteo(reading MeteoReading) {
	s.buffers.PushMeteo(reading)
}

// ProcessWind processes a wind reading
func (s *Sensor) ProcessWind(reading WindReading) {
	s.buffers.PushWind(reading)
}

// writeCSVRow writes filtered data to CSV
func (s *Sensor) writeCSVRow(data FilteredData) {
	if s.csvWriter == nil {
		return
	}

	// Get latest wind
	wind, _ := s.buffers.GetLatestWind()

	// Get latest meteo
	meteo := s.buffers.GetRecentMeteo(1)
	var tempC, pressHpa, rhPct float64
	if len(meteo) > 0 {
		tempC = meteo[0].TempC
		pressHpa = meteo[0].PressureHpa
		rhPct = meteo[0].HumidityPct
	}

	row := []string{
		data.Timestamp.UTC().Format(time.RFC3339),
		fmt.Sprintf("%.3f", float64(data.Timestamp.UnixNano())/1e9),
		fmt.Sprintf("%.6f", data.AccelX),
		fmt.Sprintf("%.6f", data.AccelY),
		fmt.Sprintf("%.6f", data.AccelZ),
		fmt.Sprintf("%.6f", data.GyroX),
		fmt.Sprintf("%.6f", data.GyroY),
		fmt.Sprintf("%.6f", data.GyroZ),
		fmt.Sprintf("%.3f", data.RollDeg),
		fmt.Sprintf("%.3f", data.PitchDeg),
		fmt.Sprintf("%.3f", data.BoomRelDeg),
		fmt.Sprintf("%.3f", data.BoomNorm),
		fmt.Sprintf("%.2f", tempC),
		fmt.Sprintf("%.2f", pressHpa),
		fmt.Sprintf("%.2f", rhPct),
		fmt.Sprintf("%.2f", wind.SpeedKts),
		fmt.Sprintf("%.2f", wind.AngleDeg),
	}

	s.csvWriter.Write(row)
	s.csvWriter.Flush()
}

// GetCurrentState returns latest sensor state
func (s *Sensor) GetCurrentState() map[string]interface{} {
	filtered := s.buffers.GetRecentFiltered(1)
	wind, _ := s.buffers.GetLatestWind()
	cal := s.calibrator.GetCalibration()

	state := map[string]interface{}{
		"has_calibration": cal != nil,
		"wind_speed_kts":  wind.SpeedKts,
		"wind_angle_deg":  wind.AngleDeg,
	}

	if len(filtered) > 0 {
		f := filtered[0]
		state["roll_deg"] = f.RollDeg
		state["pitch_deg"] = f.PitchDeg
		state["boom_rel_deg"] = f.BoomRelDeg
		state["boom_norm"] = f.BoomNorm
		state["timestamp"] = f.Timestamp.Format(time.RFC3339)
	}

	if cal != nil {
		state["calibration"] = map[string]interface{}{
			"mid":       cal.Mid,
			"span_pos":  cal.SpanPos,
			"span_neg":  cal.SpanNeg,
			"timestamp": cal.Timestamp.Format(time.RFC3339),
		}
	}

	return state
}

// GetAxisValue returns current axis value (for calibration)
func (s *Sensor) GetAxisValue() (float64, bool) {
	roll, pitch, ok := s.filter.GetState()
	if !ok {
		return 0, false
	}

	if s.config.BoomAxis == "roll" {
		return roll, true
	}
	return pitch, true
}

// RunCalibration performs interactive calibration
func (s *Sensor) RunCalibration() error {
	cal, err := s.calibrator.PerformCalibration(s.GetAxisValue)
	if err != nil {
		return err
	}

	// Save immediately
	if err := s.calibrator.SaveToFile("boom_calibration.json"); err != nil {
		log.Printf("[BoomSense] Warning: failed to save calibration: %v", err)
	}

	log.Printf("[BoomSense] Calibration complete: mid=%.2f span_pos=%.2f span_neg=%.2f",
		cal.Mid, cal.SpanPos, cal.SpanNeg)

	return nil
}

// AddEventListener registers an event callback
func (s *Sensor) AddEventListener(fn func(Event)) {
	// Wrap to add wind data enrichment
	enriched := func(evt Event) {
		// Enrich with latest wind
		if wind, ok := s.buffers.GetLatestWind(); ok {
			evt.WindSpeed = wind.SpeedKts
			evt.WindAngle = wind.AngleDeg
		}

		// Pass to original listener
		fn(evt)
	}

	s.detector.AddListener(enriched)
}

// ProcessEventFeedback performs Bayesian QA update
func (s *Sensor) ProcessEventFeedback(evt Event, isCorrect bool) {
	features := ExtractFeatures(evt)
	y := 0.0
	if isCorrect {
		y = 1.0
	}
	s.bayesian.Update(features, y, 1)

	// Save model after feedback
	if err := s.bayesian.SaveState("boom_bayes_posterior.json"); err != nil {
		log.Printf("[BoomSense] Warning: failed to save Bayesian model: %v", err)
	}
}

// EvaluateEvent returns quality probability for an event
func (s *Sensor) EvaluateEvent(evt Event) float64 {
	features := ExtractFeatures(evt)
	return s.bayesian.PredictProba(features)
}

// GetBuffers returns telemetry buffers (for visualization)
func (s *Sensor) GetBuffers() *TelemetryBuffers {
	return s.buffers
}

// GetStats returns sensor statistics
func (s *Sensor) GetStats() map[string]interface{} {
	roll, pitch, initialized := s.filter.GetState()
	cal := s.calibrator.GetCalibration()

	return map[string]interface{}{
		"filter_initialized": initialized,
		"current_roll_deg":   roll,
		"current_pitch_deg":  pitch,
		"has_calibration":    cal != nil,
		"uptime_seconds":     time.Since(s.startTime).Seconds(),
		"buffers":            s.buffers.Stats(),
	}
}