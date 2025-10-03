package boomsense_sensor

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"sync"
	"time"
)

// BoomCalibrator handles interactive boom calibration
type BoomCalibrator struct {
	boomAxis    string
	calibration *Calibration
	mu          sync.RWMutex
}

func NewBoomCalibrator(boomAxis string) *BoomCalibrator {
	return &BoomCalibrator{
		boomAxis: boomAxis,
	}
}

// PerformCalibration runs interactive 4-point calibration
// getAxisValue is a callback to retrieve current axis value
func (bc *BoomCalibrator) PerformCalibration(getAxisValue func() (float64, bool)) (*Calibration, error) {
	fmt.Println("\n=== BOOM CALIBRATION SEQUENCE ===")
	fmt.Printf("Axis for boom mapping: %s\n", bc.boomAxis)

	// Wait for filter to initialize
	if !bc.waitForFilterReady(getAxisValue, 10*time.Second) {
		return nil, fmt.Errorf("filter not initialized in time")
	}

	// Capture 4 points
	c0 := bc.capturePoint("Place BOOM CENTERED on centerline", getAxisValue)
	stb := bc.capturePoint("Ease BOOM FULLY OUT to STARBOARD (max)", getAxisValue)
	port := bc.capturePoint("Ease BOOM FULLY OUT to PORT (max)", getAxisValue)
	c1 := bc.capturePoint("Return BOOM to CENTER again (validation)", getAxisValue)

	// Calculate calibration parameters
	midExt := (stb + port) / 2.0       // Bias-resilient from extremes
	cMid := (c0 + c1) / 2.0             // Operator-defined center
	noise := math.Abs(c1 - c0)          // Larger → noisier centers

	// Adaptive blending weight
	wExt := math.Min(0.9, 0.5+noise/10.0)
	mid := wExt*midExt + (1.0-wExt)*cMid

	// Spans computed around blended mid
	spanPos := math.Max(1e-3, stb-mid)  // Starboard travel
	spanNeg := math.Max(1e-3, mid-port) // Port travel

	// Diagnostics
	off0 := c0 - mid
	off1 := c1 - mid

	fmt.Println("\n[CAL] ----------------- SUMMARY -----------------")
	fmt.Printf("[CAL] c0 (center #1): %8.3f deg\n", c0)
	fmt.Printf("[CAL] stb (max STB) : %8.3f deg\n", stb)
	fmt.Printf("[CAL] port (max PT) : %8.3f deg\n", port)
	fmt.Printf("[CAL] c1 (center #2): %8.3f deg\n", c1)
	fmt.Printf("[CAL] mid_ext (extremes): %8.3f   c_mid (centers): %8.3f\n", midExt, cMid)
	fmt.Printf("[CAL] noise|c1-c0|     : %8.3f   w_ext (auto): %4.2f\n", noise, wExt)
	fmt.Printf("[CAL] mid (blended)    : %8.3f\n", mid)
	fmt.Printf("[CAL] span_pos (STB)   : %.3f deg   span_neg (PORT): %.3f deg\n", spanPos, spanNeg)
	fmt.Printf("[CAL] center offsets vs blended mid → c0:%+.3f  c1:%+.3f\n", off0, off1)

	if math.Max(math.Abs(off0), math.Abs(off1)) > 3.0 {
		fmt.Println("\n[CAL] WARNING: Centers are >3° off blended mid. Check sea state / sensor alignment.")
	}

	fmt.Print("[CAL] Apply this calibration? [Y/n]: ")
	var ans string
	fmt.Scanln(&ans)
	if ans != "" && ans != "y" && ans != "Y" {
		fmt.Println("[CAL] Calibration aborted by user.")
		return nil, fmt.Errorf("calibration aborted")
	}

	cal := &Calibration{
		Mid:       mid,
		SpanPos:   spanPos,
		SpanNeg:   spanNeg,
		Timestamp: time.Now(),
	}

	bc.SetCalibration(cal)
	fmt.Println("[CAL] Calibration committed.")
	return cal, nil
}

// capturePoint prompts user and captures median value
func (bc *BoomCalibrator) capturePoint(instruction string, getAxisValue func() (float64, bool)) float64 {
	fmt.Println("\n----------------------------------------------------------------")
	fmt.Printf("[CAL] %s\n", instruction)
	fmt.Printf("[CAL] Keep the boom steady. Live %s value shown below.\n", bc.boomAxis)
	fmt.Println("      Press <ENTER> to capture when stable.")
	fmt.Println("----------------------------------------------------------------")

	// Live preview
	stopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				val, ok := getAxisValue()
				if ok {
					fmt.Printf("\r[CAL] %s = %7.3f deg   ", bc.boomAxis, val)
				}
			case <-stopChan:
				return
			}
		}
	}()

	// Wait for user input
	fmt.Scanln()
	close(stopChan)

	// Capture samples over 0.5s
	samples := []float64{}
	end := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(end) {
		if val, ok := getAxisValue(); ok {
			samples = append(samples, val)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Calculate median
	if len(samples) == 0 {
		fmt.Println("[CAL] No samples captured!")
		return 0.0
	}

	sort.Float64s(samples)
	median := samples[len(samples)/2]
	
	fmt.Printf("\n[CAL] Captured: %.3f deg\n", median)
	return median
}

// waitForFilterReady waits for filter initialization
func (bc *BoomCalibrator) waitForFilterReady(getAxisValue func() (float64, bool), timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := getAxisValue(); ok {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// SetCalibration updates calibration parameters
func (bc *BoomCalibrator) SetCalibration(cal *Calibration) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.calibration = cal
}

// GetCalibration returns current calibration
func (bc *BoomCalibrator) GetCalibration() *Calibration {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.calibration
}

// ComputeBoom calculates boom metrics from axis value
func (bc *BoomCalibrator) ComputeBoom(axisValue float64) (relDeg, norm float64, ok bool) {
	bc.mu.RLock()
	cal := bc.calibration
	bc.mu.RUnlock()

	if cal == nil {
		return 0, 0, false
	}

	d := axisValue - cal.Mid
	var n float64
	if d >= 0 {
		n = d / cal.SpanPos
	} else {
		n = d / cal.SpanNeg
	}
	n = math.Max(-1.1, math.Min(1.1, n))

	return d, n, true
}

// SaveToFile persists calibration to JSON
func (bc *BoomCalibrator) SaveToFile(path string) error {
	bc.mu.RLock()
	cal := bc.calibration
	bc.mu.RUnlock()

	if cal == nil {
		return fmt.Errorf("no calibration to save")
	}

	data, err := json.MarshalIndent(cal, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, data, 0644)
}

// LoadFromFile restores calibration from JSON
func (bc *BoomCalibrator) LoadFromFile(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Not an error if file doesn't exist yet
		}
		return err
	}

	var cal Calibration
	if err := json.Unmarshal(data, &cal); err != nil {
		return err
	}

	bc.SetCalibration(&cal)
	return nil
}