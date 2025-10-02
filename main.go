package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"odysail-boat-viz/integration"
	"odysail-boat-viz/nmea"
	"odysail-boat-viz/storage"
)

// Boat structures matching the JSON schema
type Boat struct {
	Name       string     `json:"name"`
	Dimensions Dimensions `json:"dimensions"`
	Polar      Polar      `json:"polar"`
	Class      string     `json:"class"`
	Metadata   Metadata   `json:"metadata"`
}

type Dimensions struct {
	LengthOverall   float64 `json:"length_overall"`
	Beam            float64 `json:"beam"`
	Draft           float64 `json:"draft"`
	Displacement    float64 `json:"displacement"`
	LengthWaterline float64 `json:"length_waterline"`
	SailAreaMain    float64 `json:"sail_area_main"`
	SailAreaJib     float64 `json:"sail_area_jib"`
	SailAreaTotal   float64 `json:"sail_area_total"`
	HullType        string  `json:"hull_type"`
	KeelType        string  `json:"keel_type"`
}

type Polar struct {
	WindSpeeds []float64   `json:"wind_speeds"`
	WindAngles []float64   `json:"wind_angles"`
	BoatSpeeds [][]float64 `json:"boat_speeds"`
}

type Metadata struct {
	P           interface{} `json:"p"`
	E           interface{} `json:"e"`
	J           interface{} `json:"j"`
	IG          interface{} `json:"ig"`
	ISP         interface{} `json:"isp"`
	Designer    string      `json:"designer"`
	Builder     string      `json:"builder"`
	Mainsails   []Mainsail  `json:"mainsails"`
	Headsails   []Headsail  `json:"headsails"`
}

type Mainsail struct {
	ID       string      `json:"id"`
	HB       interface{} `json:"hb"`
	MGT      interface{} `json:"mgt"`
	MGU      interface{} `json:"mgu"`
	MGM      interface{} `json:"mgm"`
	MGL      interface{} `json:"mgl"`
	SailArea interface{} `json:"sailarea"`
}

type Headsail struct {
	ID       string      `json:"id"`
	JH       interface{} `json:"jh"`
	JGT      interface{} `json:"jgt"`
	JGU      interface{} `json:"jgu"`
	JGM      interface{} `json:"jgm"`
	JGL      interface{} `json:"jgl"`
	LPG      interface{} `json:"lpg"`
	JibLuff  interface{} `json:"jibluff"`
	SailArea interface{} `json:"sailarea"`
}

// BoomSense sensor data structure
type BoomSenseData struct {
	BoomAngle     float64 `json:"boom_angle"`
	RollRate      float64 `json:"roll_rate"`
	PitchRate     float64 `json:"pitch_rate"`
	YawRate       float64 `json:"yaw_rate"`
	MainsheetLoad float64 `json:"mainsheet_load"`
	VangLoad      float64 `json:"vang_load"`
	EventType     string  `json:"event_type"`
	Timestamp     int64   `json:"timestamp"`
	WindSpeed     float64 `json:"wind_speed"`
	WindAngle     float64 `json:"wind_angle"`
	BoatSpeed     float64 `json:"boat_speed"`
}

// Global NMEA collector and mapper
var (
	nmeaCollector *nmea.Collector
	boomMapper    *integration.BoomSenseMapper
)

// Helper function to convert interface{} to float64
func toFloat64(val interface{}) float64 {
	if val == nil {
		return 0.0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		var f float64
		fmt.Sscanf(v, "%f", &f)
		return f
	default:
		return 0.0
	}
}

// Visualization server
type VisualizationServer struct {
	boats         []Boat
	selectedBoat  *Boat
	boomSenseData BoomSenseData
}

func NewVisualizationServer(dbPath string) (*VisualizationServer, error) {
	data, err := ioutil.ReadFile(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read database: %w", err)
	}

	var boats []Boat
	if err := json.Unmarshal(data, &boats); err != nil {
		return nil, fmt.Errorf("failed to parse database: %w", err)
	}

	return &VisualizationServer{
		boats: boats,
		boomSenseData: BoomSenseData{
			BoomAngle: 0,
			EventType: "normal",
			WindSpeed: 12.0,
			WindAngle: 45.0,
			BoatSpeed: 0.0,
		},
	}, nil
}

func (vs *VisualizationServer) SelectBoat(name string) error {
	for i := range vs.boats {
		if vs.boats[i].Name == name {
			vs.selectedBoat = &vs.boats[i]
			return nil
		}
	}
	return fmt.Errorf("boat not found: %s", name)
}

func (vs *VisualizationServer) UpdateBoomSense(data BoomSenseData) {
	vs.boomSenseData = data
}

// Generate scene data
func (vs *VisualizationServer) GenerateSceneData() map[string]interface{} {
	if vs.selectedBoat == nil {
		return map[string]interface{}{"error": "no boat selected"}
	}

	boat := vs.selectedBoat
	dim := boat.Dimensions
	meta := boat.Metadata

	boomLength := toFloat64(meta.E)
	if boomLength == 0 {
		boomLength = dim.Beam * 1.2
	}

	mastHeight := toFloat64(meta.P)
	if mastHeight == 0 {
		mastHeight = dim.LengthOverall * 1.5
	}

	return map[string]interface{}{
		"boat": map[string]interface{}{
			"name":         boat.Name,
			"length":       dim.LengthOverall,
			"beam":         dim.Beam,
			"draft":        dim.Draft,
			"displacement": dim.Displacement,
			"mastHeight":   mastHeight,
			"boomLength":   boomLength,
			"sailAreaMain": dim.SailAreaMain,
			"sailAreaJib":  dim.SailAreaJib,
			"sailAreaTotal": dim.SailAreaTotal,
			"keelType":     dim.KeelType,
			"designer":     meta.Designer,
			"builder":      meta.Builder,
		},
		"rig": map[string]interface{}{
			"p":   toFloat64(meta.P),
			"e":   toFloat64(meta.E),
			"j":   toFloat64(meta.J),
			"i":   toFloat64(meta.IG),
			"isp": toFloat64(meta.ISP),
		},
		"polar": map[string]interface{}{
			"windSpeeds": boat.Polar.WindSpeeds,
			"windAngles": boat.Polar.WindAngles,
			"boatSpeeds": boat.Polar.BoatSpeeds,
		},
		"boomSense": map[string]interface{}{
			"angle":         vs.boomSenseData.BoomAngle,
			"rollRate":      vs.boomSenseData.RollRate,
			"pitchRate":     vs.boomSenseData.PitchRate,
			"yawRate":       vs.boomSenseData.YawRate,
			"mainsheetLoad": vs.boomSenseData.MainsheetLoad,
			"vangLoad":      vs.boomSenseData.VangLoad,
			"eventType":     vs.boomSenseData.EventType,
			"timestamp":     vs.boomSenseData.Timestamp,
			"windSpeed":     vs.boomSenseData.WindSpeed,
			"windAngle":     vs.boomSenseData.WindAngle,
			"boatSpeed":     vs.boomSenseData.BoatSpeed,
		},
		"performance": vs.calculatePerformanceMetrics(),
	}
}

func (vs *VisualizationServer) calculatePerformanceMetrics() map[string]interface{} {
	if vs.selectedBoat == nil {
		return map[string]interface{}{}
	}

	optimalAngle := vs.estimateOptimalBoomAngle()
	deviation := math.Abs(vs.boomSenseData.BoomAngle - optimalAngle)

	trimEfficiency := 100.0 - (deviation / 90.0 * 100.0)
	if trimEfficiency < 0 {
		trimEfficiency = 0
	}

	targetSpeed := vs.getTargetSpeedFromPolar()

	// Calculate speed efficiency
	speedEfficiency := 100.0
	if targetSpeed > 0 && vs.boomSenseData.BoatSpeed > 0 {
		speedEfficiency = (vs.boomSenseData.BoatSpeed / targetSpeed) * 100.0
		if speedEfficiency > 100 {
			speedEfficiency = 100
		}
	}

	return map[string]interface{}{
		"optimalBoomAngle": optimalAngle,
		"deviation":        deviation,
		"trimEfficiency":   trimEfficiency,
		"speedEfficiency":  speedEfficiency,
		"alertLevel":       vs.getAlertLevel(deviation),
		"targetSpeed":      targetSpeed,
		"windSpeed":        vs.boomSenseData.WindSpeed,
		"windAngle":        vs.boomSenseData.WindAngle,
	}
}

func (vs *VisualizationServer) getTargetSpeedFromPolar() float64 {
	if vs.selectedBoat == nil || len(vs.selectedBoat.Polar.BoatSpeeds) == 0 {
		return 0.0
	}

	polar := vs.selectedBoat.Polar
	windSpeed := vs.boomSenseData.WindSpeed
	windAngle := vs.boomSenseData.WindAngle

	// Find closest wind speed index
	wsIdx := 0
	minDiff := math.Abs(polar.WindSpeeds[0] - windSpeed)
	for i, ws := range polar.WindSpeeds {
		diff := math.Abs(ws - windSpeed)
		if diff < minDiff {
			minDiff = diff
			wsIdx = i
		}
	}

	// Find closest wind angle index
	waIdx := 0
	minDiff = math.Abs(polar.WindAngles[0] - windAngle)
	for i, wa := range polar.WindAngles {
		diff := math.Abs(wa - windAngle)
		if diff < minDiff {
			minDiff = diff
			waIdx = i
		}
	}

	if wsIdx < len(polar.BoatSpeeds) && waIdx < len(polar.BoatSpeeds[wsIdx]) {
		return polar.BoatSpeeds[wsIdx][waIdx]
	}

	return 0.0
}

func (vs *VisualizationServer) estimateOptimalBoomAngle() float64 {
	windAngle := vs.boomSenseData.WindAngle
	windSpeed := vs.boomSenseData.WindSpeed

	var optimalAngle float64

	if windAngle < 45 {
		factor := 2.5 + (windSpeed / 30.0)
		optimalAngle = windAngle / factor

	} else if windAngle < 70 {
		optimalAngle = windAngle * 0.35

	} else if windAngle < 100 {
		optimalAngle = windAngle * 0.60

	} else if windAngle < 140 {
		optimalAngle = windAngle * 0.60

	} else {
		optimalAngle = 80.0
		if windSpeed < 6 {
			optimalAngle = 75.0
		}
	}

	if windSpeed < 8 {
		optimalAngle *= 0.92
	} else if windSpeed > 20 {
		optimalAngle *= 1.05
	}

	if optimalAngle < -85 {
		optimalAngle = -85
	}
	if optimalAngle > 85 {
		optimalAngle = 85
	}

	return optimalAngle
}

func (vs *VisualizationServer) getAlertLevel(deviation float64) string {
	if deviation < 5 {
		return "optimal"
	} else if deviation < 15 {
		return "good"
	} else if deviation < 30 {
		return "suboptimal"
	}
	return "poor"
}

// HTTP Handlers
func (vs *VisualizationServer) handleViewer(w http.ResponseWriter, r *http.Request) {
	html := vs.generateHTML()
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (vs *VisualizationServer) handleSceneData(w http.ResponseWriter, r *http.Request) {
	data := vs.GenerateSceneData()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (vs *VisualizationServer) handleBoatList(w http.ResponseWriter, r *http.Request) {
	searchQuery := strings.ToLower(r.URL.Query().Get("search"))
	designer := strings.ToLower(r.URL.Query().Get("designer"))
	builder := strings.ToLower(r.URL.Query().Get("builder"))

	boats := make([]map[string]interface{}, 0)
	designerSet := make(map[string]bool)
	builderSet := make(map[string]bool)

	for _, boat := range vs.boats {
		if boat.Metadata.Designer != "" {
			designerSet[boat.Metadata.Designer] = true
		}
		if boat.Metadata.Builder != "" {
			builderSet[boat.Metadata.Builder] = true
		}

		if searchQuery != "" {
			if !strings.Contains(strings.ToLower(boat.Name), searchQuery) &&
				!strings.Contains(strings.ToLower(boat.Class), searchQuery) {
				continue
			}
		}

		if designer != "" && strings.ToLower(boat.Metadata.Designer) != designer {
			continue
		}

		if builder != "" && strings.ToLower(boat.Metadata.Builder) != builder {
			continue
		}

		boats = append(boats, map[string]interface{}{
			"name":     boat.Name,
			"class":    boat.Class,
			"designer": boat.Metadata.Designer,
			"builder":  boat.Metadata.Builder,
			"length":   boat.Dimensions.LengthOverall,
		})
	}

	designers := make([]string, 0, len(designerSet))
	for d := range designerSet {
		if d != "" {
			designers = append(designers, d)
		}
	}

	builders := make([]string, 0, len(builderSet))
	for b := range builderSet {
		if b != "" {
			builders = append(builders, b)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"boats":     boats,
		"designers": designers,
		"builders":  builders,
	})
}

func (vs *VisualizationServer) handleSelectBoat(w http.ResponseWriter, r *http.Request) {
	boatName := r.URL.Query().Get("name")
	if err := vs.SelectBoat(boatName); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "selected": boatName})
}

func (vs *VisualizationServer) handleUpdateBoomSense(w http.ResponseWriter, r *http.Request) {
	var data BoomSenseData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	vs.UpdateBoomSense(data)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// NEW: NMEA API Handlers
func handleNMEAStatus(w http.ResponseWriter, r *http.Request) {
	if nmeaCollector == nil {
		http.Error(w, "NMEA collector not running", http.StatusServiceUnavailable)
		return
	}

	stats := nmeaCollector.Stats().GetSnapshot()
	bufferStats := nmeaCollector.Buffer().GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"collector": stats,
		"buffer":    bufferStats,
		"connected": nmeaCollector.IsConnected(),
	})
}

func handleNMEALatest(w http.ResponseWriter, r *http.Request) {
	if boomMapper == nil {
		http.Error(w, "BoomSense mapper not available", http.StatusServiceUnavailable)
		return
	}

	data := boomMapper.GetCurrentData()
	aws, awa := boomMapper.CalculateApparentWind()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"boomsense": data,
		"apparent_wind": map[string]float64{
			"speed": aws,
			"angle": awa,
		},
		"heel_angle": boomMapper.GetHeelAngle(),
	})
}

func handleNMEAStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if boomMapper != nil {
				data := boomMapper.GetCurrentData()
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (vs *VisualizationServer) generateHTML() string {
	// [HTML remains exactly the same as your original - not changed for brevity]
	// Copy the entire HTML string from your original file
	return `<!DOCTYPE html>
<html>
<head>
    <title>OdySail Polar Analysis - BoomSense Integration</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Segoe UI', Arial, sans-serif; background: #0a1929; color: #fff; overflow-x: hidden; }
        
        .container { display: grid; grid-template-columns: 350px 1fr 320px; height: 100vh; gap: 20px; padding: 20px; }
        
        .panel {
            background: rgba(15, 23, 42, 0.95);
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.4);
            border: 1px solid rgba(255,255,255,0.1);
            overflow-y: auto;
        }
        
        .main-panel { display: flex; flex-direction: column; gap: 20px; }
        
        h3 { 
            margin: 0 0 15px 0; 
            font-size: 16px; 
            color: #60a5fa; 
            border-bottom: 2px solid #1e40af; 
            padding-bottom: 8px; 
        }
        
        input[type="text"], select, button, input[type="number"] {
            width: 100%; 
            padding: 10px; 
            margin: 8px 0;
            border: 1px solid #334155; 
            border-radius: 6px;
            background: #1e293b; 
            color: #fff; 
            font-size: 14px;
            transition: all 0.2s;
        }
        
        input:focus, select:focus {
            outline: none; 
            border-color: #60a5fa; 
            background: #283548;
        }
        
        button {
            cursor: pointer; 
            background: #1e40af; 
            font-weight: 600;
        }
        button:hover { background: #2563eb; }
        
        .filter-label { 
            font-size: 11px; 
            color: #94a3b8; 
            text-transform: uppercase; 
            letter-spacing: 0.5px; 
            margin-bottom: 5px; 
            display: block; 
        }
        
        .boat-list {
            max-height: 250px; 
            overflow-y: auto; 
            border: 1px solid #334155; 
            border-radius: 6px;
            background: #1e293b; 
            margin: 10px 0;
        }
        
        .boat-item {
            padding: 10px; 
            cursor: pointer; 
            border-bottom: 1px solid #334155;
            transition: background 0.2s;
        }
        .boat-item:hover { background: #334155; }
        .boat-item.selected { background: #1e40af; }
        .boat-item:last-child { border-bottom: none; }
        .boat-name { font-weight: 600; color: #e2e8f0; }
        .boat-meta { font-size: 11px; color: #94a3b8; margin-top: 3px; }
        
        .metric { 
            margin: 12px 0; 
            padding: 10px;
            background: rgba(30, 41, 59, 0.5); 
            border-radius: 6px;
            border-left: 3px solid #60a5fa;
        }
        .metric-label { font-size: 11px; color: #94a3b8; text-transform: uppercase; }
        .metric-value { font-size: 20px; font-weight: bold; color: #fff; margin: 4px 0; }
        .metric-unit { font-size: 12px; color: #94a3b8; }
        
        .alert-optimal { border-left-color: #10b981; }
        .alert-good { border-left-color: #3b82f6; }
        .alert-suboptimal { border-left-color: #f59e0b; }
        .alert-poor { border-left-color: #ef4444; }
        
        .status-badge {
            display: inline-block; 
            padding: 4px 10px; 
            border-radius: 12px;
            font-size: 11px; 
            font-weight: bold;
            text-transform: uppercase;
        }
        .status-optimal { background: #10b981; color: #000; }
        .status-good { background: #3b82f6; color: #fff; }
        .status-suboptimal { background: #f59e0b; color: #000; }
        .status-poor { background: #ef4444; color: #fff; }
        
        #boat-info { font-size: 12px; color: #94a3b8; margin-top: 15px; line-height: 1.6; }
        #boat-info strong { color: #e2e8f0; }
        
        .wind-controls { 
            display: grid; 
            grid-template-columns: 1fr 1fr; 
            gap: 10px; 
            margin: 15px 0; 
        }
        .wind-input { margin: 0 !important; }
        
        #polar-container {
            background: rgba(15, 23, 42, 0.95);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255,255,255,0.1);
            min-height: 500px;
        }
        
        #polar-chart {
            width: 100%;
            height: 500px;
        }
        
        #speed-table-container {
            background: rgba(15, 23, 42, 0.95);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255,255,255,0.1);
            max-height: 400px;
            overflow: auto;
        }
        
        table {
            width: 100%;
            border-collapse: collapse;
            font-size: 12px;
        }
        
        th, td {
            padding: 8px;
            text-align: center;
            border: 1px solid #334155;
        }
        
        th {
            background: #1e40af;
            color: #fff;
            font-weight: 600;
            position: sticky;
            top: 0;
            z-index: 10;
        }
        
        td {
            background: #1e293b;
            color: #e2e8f0;
        }
        
        tr:hover td { background: #334155; }
        
        .current-condition {
            background: #10b981 !important;
            color: #000 !important;
            font-weight: bold;
        }
        
        input[type="range"] { 
            width: 100%; 
            margin: 10px 0;
            accent-color: #60a5fa;
        }
        
        .nmea-status {
            position: fixed;
            top: 20px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(15, 23, 42, 0.98);
            border: 1px solid #10b981;
            border-radius: 8px;
            padding: 8px 16px;
            font-size: 11px;
            color: #10b981;
            z-index: 1000;
            display: none;
        }
        .nmea-status.active { display: block; }
    </style>
</head>
<body>
    <div class="nmea-status" id="nmea-status">NMEA Live Data Connected</div>
    
    <div class="container">
        <!-- Left Panel: Boat Selection -->
        <div class="panel">
            <h3>⛵ Boat Selection</h3>
            
            <div class="filter-group">
                <label class="filter-label">Search Boats</label>
                <input type="text" id="search-input" placeholder="Search by name or class..." oninput="filterBoats()">
            </div>

            <div class="filter-group">
                <label class="filter-label">Filter by Designer</label>
                <select id="designer-filter" onchange="filterBoats()">
                    <option value="">All Designers</option>
                </select>
            </div>

            <div class="filter-group">
                <label class="filter-label">Filter by Builder</label>
                <select id="builder-filter" onchange="filterBoats()">
                    <option value="">All Builders</option>
                </select>
            </div>

            <div class="boat-list" id="boat-list"></div>
            
            <h3 style="margin-top: 25px;">🎮 Boom Control (Demo)</h3>
            <div class="slider-container">
                <label class="filter-label">Boom Angle (degrees)</label>
                <input type="range" id="boom-angle" min="-85" max="85" value="0" step="0.5">
                <div style="text-align: center; color: #60a5fa; font-weight: bold; font-size: 18px;" id="angle-display">0°</div>
            </div>

            <h3 style="margin-top: 25px;">💨 Wind Conditions</h3>
            <div class="wind-controls">
                <div>
                    <label class="filter-label">Wind Speed (kts)</label>
                    <input type="number" id="wind-speed" class="wind-input" value="12" min="0" max="40" step="0.5">
                </div>
                <div>
                    <label class="filter-label">Wind Angle (°)</label>
                    <input type="number" id="wind-angle" class="wind-input" value="45" min="0" max="180" step="1">
                </div>
            </div>
            
            <div>
                <label class="filter-label">Boat Speed (kts)</label>
                <input type="number" id="boat-speed" value="0" min="0" max="30" step="0.1">
            </div>
            
            <div id="boat-info"></div>
        </div>

        <!-- Center Panel: Polar Charts -->
        <div class="main-panel">
            <div id="polar-container">
                <h3>📊 Polar Diagram</h3>
                <canvas id="polar-chart"></canvas>
            </div>
            
            <div id="speed-table-container">
                <h3>📋 Speed Table (knots)</h3>
                <div id="speed-table"></div>
            </div>
        </div>

        <!-- Right Panel: Telemetry -->
        <div class="panel">
            <h3>📡 BoomSense Telemetry</h3>
            <div class="metric">
                <div class="metric-label">Current Boom Angle</div>
                <div class="metric-value"><span id="telem-angle">0</span><span class="metric-unit">°</span></div>
            </div>
            <div class="metric" id="trim-metric">
                <div class="metric-label">Trim Efficiency</div>
                <div class="metric-value"><span id="telem-efficiency">100</span><span class="metric-unit">%</span></div>
                <span class="status-badge status-optimal" id="alert-badge">OPTIMAL</span>
            </div>
            <div class="metric">
                <div class="metric-label">Optimal Boom Angle</div>
                <div class="metric-value"><span id="telem-optimal">15</span><span class="metric-unit">°</span></div>
            </div>
            <div class="metric">
                <div class="metric-label">Target Speed (Polar)</div>
                <div class="metric-value"><span id="target-speed">0.0</span><span class="metric-unit">kts</span></div>
            </div>
            <div class="metric">
                <div class="metric-label">Actual Speed</div>
                <div class="metric-value"><span id="actual-speed">0.0</span><span class="metric-unit">kts</span></div>
            </div>
            <div class="metric">
                <div class="metric-label">Speed Efficiency</div>
                <div class="metric-value"><span id="speed-efficiency">0</span><span class="metric-unit">%</span></div>
            </div>
            <div class="metric">
                <div class="metric-label">Wind Conditions</div>
                <div class="metric-value" style="font-size: 16px;"><span id="wind-display">12kts @ 45°</span></div>
            </div>
        </div>
    </div>

    <script>
        let sceneData = null;
        let allBoats = [];
        let designers = [];
        let builders = [];
        let isUpdating = false;
        let selectedBoatName = null;

        function init() {
            loadBoatList();
            
            document.getElementById('boom-angle').addEventListener('input', function(e) {
                const angle = parseFloat(e.target.value);
                document.getElementById('angle-display').textContent = angle.toFixed(1) + '°';
                updateBoomSenseData();
            });

            document.getElementById('wind-speed').addEventListener('change', updateWindConditions);
            document.getElementById('wind-angle').addEventListener('change', updateWindConditions);
            document.getElementById('boat-speed').addEventListener('change', updateWindConditions);
            
            // Connect to NMEA live stream
            connectNMEAStream();
        }

        function connectNMEAStream() {
            const stream = new EventSource('/api/nmea/stream');
            
            stream.onopen = () => {
                console.log('[NMEA] Live data connected');
                document.getElementById('nmea-status').classList.add('active');
                setTimeout(() => {
                    document.getElementById('nmea-status').classList.remove('active');
                }, 3000);
            };
            
            stream.onmessage = (event) => {
                const data = JSON.parse(event.data);
                
                // Auto-fill wind conditions from live data
                if (data.wind_speed > 0) {
                    document.getElementById('wind-speed').value = data.wind_speed.toFixed(1);
                }
                if (data.wind_angle > 0) {
                    document.getElementById('wind-angle').value = data.wind_angle.toFixed(0);
                }
                
                // Auto-fill boat speed from live data
                if (data.boat_speed > 0) {
                    document.getElementById('boat-speed').value = data.boat_speed.toFixed(1);
                }
                
                // Trigger UI update with live data
                updateWindConditions();
            };
            
            stream.onerror = () => {
                console.log('[NMEA] Connection lost, retrying in 5s...');
                setTimeout(connectNMEAStream, 5000);
            };
        }

        function loadBoatList() {
            fetch('/api/boats')
                .then(r => r.json())
                .then(data => {
                    allBoats = data.boats;
                    designers = data.designers;
                    builders = data.builders;
                    
                    populateFilters();
                    displayBoats(allBoats);
                });
        }

        function populateFilters() {
            const designerSelect = document.getElementById('designer-filter');
            designers.sort().forEach(d => {
                const option = document.createElement('option');
                option.value = d.toLowerCase();
                option.textContent = d;
                designerSelect.appendChild(option);
            });

            const builderSelect = document.getElementById('builder-filter');
            builders.sort().forEach(b => {
                const option = document.createElement('option');
                option.value = b.toLowerCase();
                option.textContent = b;
                builderSelect.appendChild(option);
            });
        }

        function filterBoats() {
            const search = document.getElementById('search-input').value.toLowerCase();
            const designer = document.getElementById('designer-filter').value;
            const builder = document.getElementById('builder-filter').value;

            let filtered = allBoats;

            if (search) {
                filtered = filtered.filter(b => 
                    b.name.toLowerCase().includes(search) || 
                    b.class.toLowerCase().includes(search)
                );
            }

            if (designer) {
                filtered = filtered.filter(b => b.designer.toLowerCase() === designer);
            }

            if (builder) {
                filtered = filtered.filter(b => b.builder.toLowerCase() === builder);
            }

            displayBoats(filtered);
        }

        function displayBoats(boats) {
            const listEl = document.getElementById('boat-list');
            listEl.innerHTML = '';

            if (boats.length === 0) {
                listEl.innerHTML = '<div style="padding: 20px; text-align: center; color: #94a3b8;">No boats found</div>';
                return;
            }

            boats.forEach(boat => {
                const item = document.createElement('div');
                item.className = 'boat-item';
                if (boat.name === selectedBoatName) {
                    item.classList.add('selected');
                }
                item.innerHTML = 
                    '<div class="boat-name">' + boat.name + '</div>' +
                    '<div class="boat-meta">' + boat.class + ' | ' + boat.length.toFixed(2) + 'm | ' + boat.designer + '</div>';
                item.onclick = () => selectBoat(boat.name);
                listEl.appendChild(item);
            });
        }

        function selectBoat(boatName) {
            selectedBoatName = boatName;
            fetch('/api/select?name=' + encodeURIComponent(boatName))
                .then(r => r.json())
                .then(() => loadSceneData())
                .catch(err => console.error('Error:', err));
        }

        function loadSceneData() {
            fetch('/api/scene')
                .then(r => r.json())
                .then(data => {
                    sceneData = data;
                    updateBoatInfo(data);
                    updateTelemetry(data);
                    drawPolarChart(data);
                    createSpeedTable(data);
                    displayBoats(allBoats);
                });
        }

        function drawPolarChart(data) {
            if (!data.polar || !data.polar.windAngles || !data.polar.boatSpeeds) return;

            const canvas = document.getElementById('polar-chart');
            const ctx = canvas.getContext('2d');
            
            canvas.width = canvas.offsetWidth;
            canvas.height = 500;

            const centerX = canvas.width / 2;
            const centerY = canvas.height / 2;
            const maxRadius = Math.min(centerX, centerY) - 60;

            ctx.clearRect(0, 0, canvas.width, canvas.height);

            // Find max speed for scaling
            let maxSpeed = 0;
            data.polar.boatSpeeds.forEach(speeds => {
                speeds.forEach(speed => {
                    if (speed > maxSpeed) maxSpeed = speed;
                });
            });

            // Draw concentric circles (speed rings)
            ctx.strokeStyle = '#334155';
            ctx.lineWidth = 1;
            const speedSteps = 5;
            for (let i = 1; i <= speedSteps; i++) {
                const radius = (maxRadius / speedSteps) * i;
                ctx.beginPath();
                ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
                ctx.stroke();
                
                // Speed labels
                ctx.fillStyle = '#94a3b8';
                ctx.font = '10px sans-serif';
                ctx.fillText((maxSpeed / speedSteps * i).toFixed(1) + ' kts', centerX + 5, centerY - radius);
            }

            // Draw angle lines
            ctx.strokeStyle = '#334155';
            data.polar.windAngles.forEach(angle => {
                const rad = (angle - 90) * Math.PI / 180;
                ctx.beginPath();
                ctx.moveTo(centerX, centerY);
                ctx.lineTo(
                    centerX + Math.cos(rad) * maxRadius,
                    centerY + Math.sin(rad) * maxRadius
                );
                ctx.stroke();
                
                // Angle labels
                ctx.fillStyle = '#94a3b8';
                ctx.font = '11px sans-serif';
                ctx.fillText(angle.toFixed(0) + '°', 
                    centerX + Math.cos(rad) * (maxRadius + 20),
                    centerY + Math.sin(rad) * (maxRadius + 20)
                );
            });

            // Draw polar curves for each wind speed
            const colors = ['#ef4444', '#f59e0b', '#10b981', '#3b82f6', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#f43f5e'];
            
            data.polar.windSpeeds.forEach((windSpeed, wsIdx) => {
                if (wsIdx >= data.polar.boatSpeeds.length) return;
                
                ctx.strokeStyle = colors[wsIdx % colors.length];
                ctx.lineWidth = 2;
                ctx.beginPath();
                
                let first = true;
                data.polar.windAngles.forEach((angle, waIdx) => {
                    const speed = data.polar.boatSpeeds[wsIdx][waIdx];
                    const radius = (speed / maxSpeed) * maxRadius;
                    const rad = (angle - 90) * Math.PI / 180;
                    
                    const x = centerX + Math.cos(rad) * radius;
                    const y = centerY + Math.sin(rad) * radius;
                    
                    if (first) {
                        ctx.moveTo(x, y);
                        first = false;
                    } else {
                        ctx.lineTo(x, y);
                    }
                });
                
                ctx.stroke();
            });

            // Draw legend
            let legendY = 20;
            data.polar.windSpeeds.forEach((windSpeed, idx) => {
                ctx.fillStyle = colors[idx % colors.length];
                ctx.fillRect(20, legendY, 15, 15);
                ctx.fillStyle = '#e2e8f0';
                ctx.font = '12px sans-serif';
                ctx.fillText(windSpeed.toFixed(0) + ' kts', 40, legendY + 12);
                legendY += 20;
            });

            // Draw current condition marker
            if (data.boomSense && data.boomSense.windAngle && data.boomSense.windSpeed) {
                const targetSpeed = data.performance.targetSpeed;
                const radius = (targetSpeed / maxSpeed) * maxRadius;
                const rad = (data.boomSense.windAngle - 90) * Math.PI / 180;
                
                ctx.fillStyle = '#10b981';
                ctx.beginPath();
                ctx.arc(
                    centerX + Math.cos(rad) * radius,
                    centerY + Math.sin(rad) * radius,
                    8, 0, Math.PI * 2
                );
                ctx.fill();
                
                ctx.strokeStyle = '#fff';
                ctx.lineWidth = 2;
                ctx.stroke();
            }
        }

        function createSpeedTable(data) {
            if (!data.polar || !data.polar.windAngles || !data.polar.boatSpeeds) return;

            const container = document.getElementById('speed-table');
            let html = '<table><thead><tr><th>TWA \\ TWS</th>';
            
            data.polar.windSpeeds.forEach(ws => {
                html += '<th>' + ws.toFixed(0) + ' kts</th>';
            });
            html += '</tr></thead><tbody>';

            data.polar.windAngles.forEach((angle, waIdx) => {
                html += '<tr><td><strong>' + angle.toFixed(0) + '°</strong></td>';
                
                data.polar.boatSpeeds.forEach((speeds, wsIdx) => {
                    const speed = speeds[waIdx];
                    const isCurrent = Math.abs(angle - data.boomSense.windAngle) < 5 && 
                                     Math.abs(data.polar.windSpeeds[wsIdx] - data.boomSense.windSpeed) < 2;
                    const className = isCurrent ? 'current-condition' : '';
                    html += '<td class="' + className + '">' + speed.toFixed(2) + '</td>';
                });
                html += '</tr>';
            });

            html += '</tbody></table>';
            container.innerHTML = html;
        }

        function updateBoomSenseData() {
            if (isUpdating) return;
            isUpdating = true;

            const angle = parseFloat(document.getElementById('boom-angle').value);
            const windSpeed = parseFloat(document.getElementById('wind-speed').value);
            const windAngle = parseFloat(document.getElementById('wind-angle').value);
            const boatSpeed = parseFloat(document.getElementById('boat-speed').value);

            fetch('/api/boomsense', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    boom_angle: angle,
                    event_type: 'normal',
                    timestamp: Date.now(),
                    wind_speed: windSpeed,
                    wind_angle: windAngle,
                    boat_speed: boatSpeed
                })
            }).then(() => {
                return fetch('/api/scene');
            }).then(r => r.json())
            .then(data => {
                updateTelemetry(data);
                drawPolarChart(data);
                createSpeedTable(data);
                isUpdating = false;
            });
        }

        function updateWindConditions() {
            updateBoomSenseData();
        }

        function updateBoatInfo(data) {
            const b = data.boat;
            const sqLabel = 'm²';
            document.getElementById('boat-info').innerHTML = 
                '<strong>Name:</strong> ' + b.name + '<br>' +
                '<strong>Designer:</strong> ' + b.designer + '<br>' +
                '<strong>Builder:</strong> ' + b.builder + '<br>' +
                '<strong>Length:</strong> ' + b.length.toFixed(2) + 'm<br>' +
                '<strong>Beam:</strong> ' + b.beam.toFixed(2) + 'm<br>' +
                '<strong>Draft:</strong> ' + b.draft.toFixed(2) + 'm<br>' +
                '<strong>Displacement:</strong> ' + b.displacement.toFixed(0) + 'kg<br>' +
                '<strong>Sail Area:</strong> ' + b.sailAreaTotal.toFixed(1) + sqLabel;
        }

        function updateTelemetry(data) {
            const bs = data.boomSense;
            const perf = data.performance;
            
            document.getElementById('telem-angle').textContent = bs.angle.toFixed(1);
            document.getElementById('telem-efficiency').textContent = perf.trimEfficiency.toFixed(1);
            document.getElementById('telem-optimal').textContent = perf.optimalBoomAngle.toFixed(1);
            document.getElementById('target-speed').textContent = perf.targetSpeed.toFixed(2);
            document.getElementById('actual-speed').textContent = bs.boatSpeed.toFixed(2);
            document.getElementById('speed-efficiency').textContent = perf.speedEfficiency.toFixed(1);
            document.getElementById('wind-display').textContent = perf.windSpeed.toFixed(1) + 'kts @ ' + perf.windAngle.toFixed(0) + '°';

            const badge = document.getElementById('alert-badge');
            const metric = document.getElementById('trim-metric');
            badge.className = 'status-badge status-' + perf.alertLevel;
            badge.textContent = perf.alertLevel.toUpperCase();
            metric.className = 'metric alert-' + perf.alertLevel;
        }

        init();
    </script>
</body>
</html>`
}

func main() {
	dbPath := "orc_boat_db.json"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	server, err := NewVisualizationServer(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Initialize NMEA collector
	log.Printf("[NMEA] Initializing collector...")
	nmeaConfig := nmea.DefaultConfig()
	buffer := storage.NewRingBuffer(nmeaConfig.BufferSize)

	var csvWriter *storage.CSVWriter
	if nmeaConfig.EnableCSV {
		csvWriter = storage.NewCSVWriter(
			nmeaConfig.CSVFramesPath,
			nmeaConfig.CSVDecodedPath,
			nmeaConfig.CSVStatsPath,
		)
	}

	nmeaCollector = nmea.NewCollector(nmeaConfig, buffer, csvWriter)

	if err := nmeaCollector.Start(); err != nil {
		log.Printf("[WARN] NMEA collector failed to start: %v", err)
		log.Printf("[WARN] Running without live N2K data")
	} else {
		log.Printf("[NMEA] Collector started successfully")
		defer nmeaCollector.Stop()
	}

	// Initialize BoomSense mapper
	boomMapper = integration.NewBoomSenseMapper(buffer)

	// Setup HTTP routes
	http.HandleFunc("/", server.handleViewer)
	http.HandleFunc("/api/scene", server.handleSceneData)
	http.HandleFunc("/api/boats", server.handleBoatList)
	http.HandleFunc("/api/select", server.handleSelectBoat)
	http.HandleFunc("/api/boomsense", server.handleUpdateBoomSense)

	// NMEA API endpoints
	http.HandleFunc("/api/nmea/status", handleNMEAStatus)
	http.HandleFunc("/api/nmea/latest", handleNMEALatest)
	http.HandleFunc("/api/nmea/stream", handleNMEAStream)

	port := ":8080"
	fmt.Printf("🚢 OdySail Polar Analysis Server\n")
	fmt.Printf("📡 BoomSense Integration Active\n")
	fmt.Printf("🌐 Server running at http://localhost%s\n", port)
	fmt.Printf("📊 Loaded %d boats from database\n", len(server.boats))
	if nmeaCollector != nil && nmeaCollector.IsConnected() {
		fmt.Printf("✅ NMEA2000 collector connected\n")
	}
	fmt.Println()

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}