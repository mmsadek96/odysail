package boomsense_sensor

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"sync"
)

// BayesianQA implements online Bayesian logistic regression for event quality assessment
type BayesianQA struct {
	d     int       // Feature dimension
	mu    []float64 // Mean weights
	vr    []float64 // Variance (diagonal)
	lock  sync.RWMutex
}

// NewBayesianQA creates a new Bayesian QA model
func NewBayesianQA(d int, sigma0 float64) *BayesianQA {
	mu := make([]float64, d)
	vr := make([]float64, d)
	for i := 0; i < d; i++ {
		vr[i] = sigma0 * sigma0
	}
	
	return &BayesianQA{
		d:  d,
		mu: mu,
		vr: vr,
	}
}

// PredictProba returns probability that event is correct
func (bq *BayesianQA) PredictProba(x []float64) float64 {
	bq.lock.RLock()
	defer bq.lock.RUnlock()

	if len(x) != bq.d {
		return 0.5 // Invalid feature vector
	}

	// Mean prediction: m = mu · x
	m := 0.0
	for i := 0; i < bq.d; i++ {
		m += bq.mu[i] * x[i]
	}

	// Variance: s² = Σ(var_i * x_i²)
	s2 := 0.0
	for i := 0; i < bq.d; i++ {
		s2 += bq.vr[i] * x[i] * x[i]
	}

	// Probit approximation for Bayesian logistic regression
	k := 1.0 / math.Sqrt(1.0+math.Pi*s2/8.0)
	z := k * m

	// Sigmoid
	return sigmoid(z)
}

// Update performs online Bayesian update
func (bq *BayesianQA) Update(x []float64, y float64, iters int) {
	bq.lock.Lock()
	defer bq.lock.Unlock()

	if len(x) != bq.d {
		return
	}

	for iter := 0; iter < iters; iter++ {
		// Forward pass
		z := 0.0
		for i := 0; i < bq.d; i++ {
			z += bq.mu[i] * x[i]
		}
		p := sigmoid(z)

		// Gradient and Hessian (diagonal approximation)
		for i := 0; i < bq.d; i++ {
			g := (y - p) * x[i]
			h := p*(1-p)*x[i]*x[i] + 1.0/bq.vr[i]
			
			// Update mean
			step := g / h
			bq.mu[i] += step
			
			// Update variance (diagonal approximation)
			bq.vr[i] = 1.0 / h
		}
	}
}

// ExtractFeatures converts event to feature vector
// Feature vector (11 dimensions with wind):
// [gy_peak, boom_delta, dt, roll_delta, overshoot, 
//  is_tack, is_gybe_normal, is_gybe_crash,
//  wind_speed_kn, wind_angle_deg, bias]
func ExtractFeatures(evt Event) []float64 {
	// Extract raw features
	gy := evt.GyroPeak
	bd := evt.BoomDelta
	dt := evt.Duration
	rl := evt.RollDelta
	os := evt.Overshoot
	ws := evt.WindSpeed
	wa := evt.WindAngle

	// One-hot encode event type
	tTack := 0.0
	tGN := 0.0
	tGC := 0.0
	switch evt.Type {
	case "tack":
		tTack = 1.0
	case "gybe_normal":
		tGN = 1.0
	case "gybe_crash":
		tGC = 1.0
	}

	// Build feature vector
	x := []float64{gy, bd, dt, rl, os, tTack, tGN, tGC, ws, wa, 1.0}

	// Scale features (matching Python scales)
	scales := []float64{150, 1.5, 2.5, 25, 0.4, 1, 1, 1, 40, 180, 1}
	for i := 0; i < len(x); i++ {
		x[i] /= scales[i]
	}

	return x
}

// sigmoid computes logistic sigmoid
func sigmoid(z float64) float64 {
	z = math.Max(-60.0, math.Min(60.0, z)) // Prevent overflow
	return 1.0 / (1.0 + math.Exp(-z))
}

// SaveState persists model to JSON
func (bq *BayesianQA) SaveState(path string) error {
	bq.lock.RLock()
	defer bq.lock.RUnlock()

	state := map[string]interface{}{
		"mu":  bq.mu,
		"var": bq.vr,
		"d":   bq.d,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, data, 0644)
}

// LoadState restores model from JSON
func (bq *BayesianQA) LoadState(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	bq.lock.Lock()
	defer bq.lock.Unlock()

	// Load mu
	if muData, ok := state["mu"].([]interface{}); ok {
		bq.mu = make([]float64, len(muData))
		for i, v := range muData {
			if f, ok := v.(float64); ok {
				bq.mu[i] = f
			}
		}
	}

	// Load var
	if vrData, ok := state["var"].([]interface{}); ok {
		bq.vr = make([]float64, len(vrData))
		for i, v := range vrData {
			if f, ok := v.(float64); ok {
				bq.vr[i] = f
			}
		}
	}

	return nil
}