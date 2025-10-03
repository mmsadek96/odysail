package boomsense_sensor

import (
	"math"
	"sync"
	"time"
)

// EventDetector implements rule-based sailing event detection
type EventDetector struct {
	config          Config
	buffer          []eventSample
	maxBufferSize   int
	lastEventTime   float64
	listeners       []func(Event)
	mu              sync.RWMutex
}

type eventSample struct {
	t        float64
	gyro     float64
	boomNorm float64
	roll     float64
}

func NewEventDetector(config Config) *EventDetector {
	return &EventDetector{
		config:        config,
		buffer:        make([]eventSample, 0, config.MaxBufferSize),
		maxBufferSize: config.MaxBufferSize,
		lastEventTime: -1e9,
	}
}

// AddListener registers an event callback
func (ed *EventDetector) AddListener(fn func(Event)) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.listeners = append(ed.listeners, fn)
}

// OnSample processes a new sensor sample
func (ed *EventDetector) OnSample(t time.Time, gyroY, boomNorm, roll float64) {
	ed.mu.Lock()
	defer ed.mu.Unlock()

	ts := float64(t.UnixNano()) / 1e9

	// Add to buffer
	sample := eventSample{
		t:        ts,
		gyro:     gyroY,
		boomNorm: boomNorm,
		roll:     roll,
	}

	ed.buffer = append(ed.buffer, sample)
	if len(ed.buffer) > ed.maxBufferSize {
		ed.buffer = ed.buffer[1:]
	}

	// Check for events
	ed.maybeEmit(ts)
}

// maybeEmit checks conditions and emits events
func (ed *EventDetector) maybeEmit(tNow float64) {
	// Refractory period
	if (tNow - ed.lastEventTime) < ed.config.RefractoryPeriod {
		return
	}

	// Check crash gybe
	if evt := ed.checkCrashGybe(tNow); evt != nil {
		ed.publish(*evt)
		return
	}

	// Check normal gybe
	if evt := ed.checkNormalGybe(tNow); evt != nil {
		ed.publish(*evt)
		return
	}

	// Check tack
	if evt := ed.checkTack(tNow); evt != nil {
		ed.publish(*evt)
		return
	}

	// Check boom hit
	if evt := ed.checkBoomHit(tNow); evt != nil {
		ed.publish(*evt)
		return
	}
}

// checkCrashGybe detects crash gybes
func (ed *EventDetector) checkCrashGybe(tNow float64) *Event {
	dt, gyPeak, boomDelta, rollDrop, _, _ := ed.spanIn(tNow, ed.config.CrashDT)

	if gyPeak >= ed.config.CrashGyDPS && boomDelta >= ed.config.BoomStepCrash {
		return &Event{
			Type:      "gybe_crash",
			Timestamp: time.Unix(0, int64(tNow*1e9)),
			GyroPeak:  gyPeak,
			BoomDelta: boomDelta,
			RollDelta: rollDrop,
			Duration:  dt,
		}
	}
	return nil
}

// checkNormalGybe detects normal gybes
func (ed *EventDetector) checkNormalGybe(tNow float64) *Event {
	dt, gyPeak, boomDelta, rollDrop, _, _ := ed.spanIn(tNow, ed.config.NormalDT)

	if gyPeak >= ed.config.NormalGyMin && 
	   gyPeak < ed.config.CrashGyDPS && 
	   boomDelta >= ed.config.BoomStepNormal {
		return &Event{
			Type:      "gybe_normal",
			Timestamp: time.Unix(0, int64(tNow*1e9)),
			GyroPeak:  gyPeak,
			BoomDelta: boomDelta,
			RollDelta: rollDrop,
			Duration:  dt,
		}
	}
	return nil
}

// checkTack detects tacks
func (ed *EventDetector) checkTack(tNow float64) *Event {
	dt, gyPeak, boomDelta, rollDrop, bnSeries, _ := ed.spanIn(tNow, ed.config.TackDTMax)

	if gyPeak >= ed.config.TackGyMin && 
	   gyPeak <= ed.config.TackGyMax &&
	   boomDelta >= ed.config.TackBoomStep &&
	   rollDrop >= ed.config.TackMinRollDelta {

		direction := ed.tackDirection(bnSeries)
		overshoot := ed.calculateOvershoot(bnSeries)
		score := ed.tackQualityScore(dt, gyPeak, rollDrop, overshoot)

		return &Event{
			Type:      "tack",
			Timestamp: time.Unix(0, int64(tNow*1e9)),
			Direction: direction,
			GyroPeak:  gyPeak,
			BoomDelta: boomDelta,
			RollDelta: rollDrop,
			Duration:  dt,
			Overshoot: overshoot,
			Score:     score,
		}
	}
	return nil
}

// checkBoomHit detects boom hits
func (ed *EventDetector) checkBoomHit(tNow float64) *Event {
	dt, gyPeak, _, rollDrop, _, _ := ed.spanIn(tNow, ed.config.RollDT)

	if gyPeak >= (ed.config.CrashGyDPS+20) && rollDrop >= ed.config.RollHit {
		return &Event{
			Type:      "boom_hit",
			Timestamp: time.Unix(0, int64(tNow*1e9)),
			GyroPeak:  gyPeak,
			RollDelta: rollDrop,
			Duration:  dt,
		}
	}
	return nil
}

// spanIn extracts statistics over a time window
func (ed *EventDetector) spanIn(tNow, horizon float64) (dt, gyPeak, boomDelta, rollDrop float64, bnSeries, rlSeries []float64) {
	t0 := math.Max(0, tNow-horizon)
	
	var sub []eventSample
	for _, s := range ed.buffer {
		if s.t >= t0 {
			sub = append(sub, s)
		}
	}

	if len(sub) == 0 {
		return 0, 0, 0, 0, nil, nil
	}

	dt = sub[len(sub)-1].t - sub[0].t

	// Gyro peak (absolute)
	for _, s := range sub {
		if abs := math.Abs(s.gyro); abs > gyPeak {
			gyPeak = abs
		}
	}

	// Boom delta
	var bnValid []float64
	for _, s := range sub {
		if math.IsInf(s.boomNorm, 0) || math.IsNaN(s.boomNorm) {
			continue
		}
		bnValid = append(bnValid, s.boomNorm)
		bnSeries = append(bnSeries, s.boomNorm)
	}
	if len(bnValid) >= 2 {
		minBn, maxBn := bnValid[0], bnValid[0]
		for _, v := range bnValid {
			if v < minBn {
				minBn = v
			}
			if v > maxBn {
				maxBn = v
			}
		}
		boomDelta = maxBn - minBn
	}

	// Roll drop (max decrease)
	var rlValid []float64
	for _, s := range sub {
		if math.IsInf(s.roll, 0) || math.IsNaN(s.roll) {
			continue
		}
		rlValid = append(rlValid, s.roll)
		rlSeries = append(rlSeries, s.roll)
	}
	if len(rlValid) >= 2 {
		for i := 0; i < len(rlValid); i++ {
			for j := i + 1; j < len(rlValid); j++ {
				drop := rlValid[i] - rlValid[j]
				if drop > rollDrop {
					rollDrop = drop
				}
			}
		}
	}

	return
}

// tackDirection determines tack direction
func (ed *EventDetector) tackDirection(bnSeries []float64) string {
	if len(bnSeries) < 2 {
		return ""
	}
	if bnSeries[0] > 0 && bnSeries[len(bnSeries)-1] < 0 {
		return "stb_to_port"
	}
	if bnSeries[0] < 0 && bnSeries[len(bnSeries)-1] > 0 {
		return "port_to_stb"
	}
	return ""
}

// calculateOvershoot measures tack overshoot
func (ed *EventDetector) calculateOvershoot(bnSeries []float64) float64 {
	if len(bnSeries) < 4 {
		return 0.0
	}

	// Head: first quarter average (unused but calculated for consistency)
	headN := int(math.Max(2, float64(len(bnSeries))/4))
	headSum := 0.0
	for i := 0; i < headN; i++ {
		headSum += bnSeries[i]
	}
	_ = headSum / float64(headN) // head unused in current algorithm

	// Tail: last quarter average
	tailN := int(math.Max(2, float64(len(bnSeries))/4))
	tailSum := 0.0
	for i := len(bnSeries) - tailN; i < len(bnSeries); i++ {
		tailSum += bnSeries[i]
	}
	tail := tailSum / float64(tailN)

	// Overshoot: max deviation in last third from target
	target := tail
	tailThird := int(math.Max(5, float64(len(bnSeries))/3))
	maxDev := 0.0
	for i := len(bnSeries) - tailThird; i < len(bnSeries); i++ {
		dev := math.Abs(bnSeries[i] - target)
		if dev > maxDev {
			maxDev = dev
		}
	}

	return math.Max(0.0, maxDev-0.05)
}

// tackQualityScore calculates tack quality (0-100)
func (ed *EventDetector) tackQualityScore(dt, gyPeak, rollDrop, overshoot float64) float64 {
	// Time component (faster is better, target ~1.6s)
	tComp := math.Max(0, 40*(1.6/math.Max(0.6, dt)))

	// Gyro component (smooth ~55 deg/s is optimal)
	gyComp := math.Max(0, 30*(1.0-math.Abs(gyPeak-55)/55))

	// Roll component (more heel change is better, up to 25 deg)
	rlComp := math.Max(0, 20*(math.Min(rollDrop, 25)/25))

	// Overshoot penalty (less is better)
	osComp := math.Max(0, 10*(1.0-math.Min(overshoot/0.25, 1.0)))

	score := tComp + gyComp + rlComp + osComp
	return math.Min(100, math.Round(score*10)/10)
}

// publish notifies all listeners
func (ed *EventDetector) publish(evt Event) {
	ed.lastEventTime = float64(evt.Timestamp.UnixNano()) / 1e9
	
	for _, fn := range ed.listeners {
		go func(f func(Event)) {
			defer func() {
				if r := recover(); r != nil {
					// Listener panicked, ignore
				}
			}()
			f(evt)
		}(fn)
	}
}