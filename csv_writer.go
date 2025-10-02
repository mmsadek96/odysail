package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DecodedMessage is a local copy to avoid circular import
type DecodedMessage struct {
	Timestamp   time.Time
	PGN         int
	PGNName     string
	Source      uint8
	Measurement string
	Fields      map[string]interface{}
	Raw         []byte
}

type CSVWriter struct {
	framesFile  *os.File
	decodedFile *os.File
	statsFile   *os.File

	framesWriter  *csv.Writer
	decodedWriter *csv.Writer
	statsWriter   *csv.Writer
}

func NewCSVWriter(framesPath, decodedPath, statsPath string) *CSVWriter {
	// Create data directory if needed
	os.MkdirAll(filepath.Dir(framesPath), 0755)

	w := &CSVWriter{}

	// Open files
	w.framesFile, _ = os.OpenFile(framesPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	w.decodedFile, _ = os.OpenFile(decodedPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	w.statsFile, _ = os.OpenFile(statsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)

	w.framesWriter = csv.NewWriter(w.framesFile)
	w.decodedWriter = csv.NewWriter(w.decodedFile)
	w.statsWriter = csv.NewWriter(w.statsFile)

	// Write headers if files are new
	w.writeHeaders()

	return w
}

func (w *CSVWriter) writeHeaders() {
	// Check file size, if 0 write headers
	info, _ := w.decodedFile.Stat()
	if info.Size() == 0 {
		w.decodedWriter.Write([]string{
			"iso8601", "ts_ms", "measurement", "pgn", "pgn_name",
			"source", "field", "value",
		})
		w.decodedWriter.Flush()
	}
}

func (w *CSVWriter) WriteDecoded(msg DecodedMessage) {
	for field, value := range msg.Fields {
		row := []string{
			msg.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", msg.Timestamp.UnixMilli()),
			msg.Measurement,
			fmt.Sprintf("%d", msg.PGN),
			msg.PGNName,
			fmt.Sprintf("%d", msg.Source),
			field,
			fmt.Sprintf("%v", value),
		}
		w.decodedWriter.Write(row)
	}
	w.decodedWriter.Flush()
}

func (w *CSVWriter) Close() {
	if w.framesWriter != nil {
		w.framesWriter.Flush()
		w.framesFile.Close()
	}
	if w.decodedWriter != nil {
		w.decodedWriter.Flush()
		w.decodedFile.Close()
	}
	if w.statsWriter != nil {
		w.statsWriter.Flush()
		w.statsFile.Close()
	}
}