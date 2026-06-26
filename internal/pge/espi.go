// espi.go parses PG&E's ESPI (Energy Service Provider Interface) Atom/XML feed
// into plain Reading structs.
//
// Relevant ESPI concepts:
//
//	IntervalBlock        — a contiguous run of readings (typically one day)
//	IntervalReading      — one interval (~15 minutes for PG&E electric; the
//	                       sandbox feed uses 899 s, so we trust the per-reading
//	                       duration rather than assuming exactly 900 s)
//	ReadingType          — metadata for the readings that follow it: the scale
//	                       factor (powerOfTenMultiplier), unit (uom: 72 = Wh,
//	                       38 = W), measurement kind (12 = energy, 8 = demand),
//	                       and flowDirection (1 = import, 19 = export)
//
// Flow-direction attribution: PG&E emits, per metered channel, a ReadingType
// entry immediately followed by its IntervalBlock entries. We therefore apply the
// most recently seen ReadingType to each IntervalBlock. This matches PG&E's
// ordering; a feed that front-loaded all ReadingTypes before all IntervalBlocks
// would misattribute, but PG&E does not produce that shape.
//
// Energy-only: a single feed can carry non-energy channels — e.g. a demand
// register reporting watts (uom 38, kind 8, flowDirection 0). Those are not kWh
// and would corrupt the series if summed as energy, so we skip any IntervalBlock
// whose governing ReadingType is not an energy reading in Wh.
//
// Signed values: the import channel may itself carry negative readings (a net
// register where export exceeds import in an interval). We preserve that sign —
// a negative import reading is a net export — and only negate the dedicated
// received channel (flowDirection 19), which reports export as a positive
// magnitude.
//
// Known limitation: a feed can contain more than one energy UsagePoint in the
// same direction (the sandbox feed pairs a gross-import register with a signed
// net register, both flowDirection 1). We key on flowDirection, not UsagePoint,
// so such registers are concatenated rather than de-duplicated. Real PG&E Share
// My Data feeds expose one energy register per direction, so this does not arise
// in production; revisit by threading UsagePoint identity if it ever does.
package pge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ESPI flowDirection codes.
const (
	flowDelivered = 1  // import: energy supplied to the customer
	flowReceived  = 19 // export: generation flowing back to the grid
)

// ESPI unit-of-measure and kind codes we care about.
const (
	uomWh      = 72 // watt-hours: the only unit we treat as energy
	kindEnergy = 12 // measurement kind: energy (vs. 8 = demand/power)
)

// ---- ESPI XML structs -------------------------------------------------------

type feed struct {
	XMLName xml.Name `xml:"feed"`
	Entries []entry  `xml:"entry"`
}

type entry struct {
	Content content `xml:"content"`
}

type content struct {
	IntervalBlock *intervalBlock `xml:"IntervalBlock"`
	ReadingType   *readingType   `xml:"ReadingType"`
}

type readingType struct {
	FlowDirection        int `xml:"flowDirection"`
	PowerOfTenMultiplier int `xml:"powerOfTenMultiplier"`
	UOM                  int `xml:"uom"`            // 72 = Wh, 38 = W
	Kind                 int `xml:"kind"`           // 12 = energy, 8 = demand
	IntervalLength       int `xml:"intervalLength"` // seconds; ~900 = 15 min
}

type intervalBlock struct {
	Interval         timeInterval      `xml:"interval"`
	IntervalReadings []intervalReading `xml:"IntervalReading"`
}

type timeInterval struct {
	Duration int64 `xml:"duration"`
	Start    int64 `xml:"start"` // Unix epoch seconds
}

type intervalReading struct {
	TimePeriod timeInterval `xml:"timePeriod"`
	Value      int64        `xml:"value"` // raw value; apply powerOfTenMultiplier
}

// ---- Parsed output ----------------------------------------------------------

// Reading is one interval with a timestamp and signed energy value: positive =
// import (bought from grid), negative = export (sold to grid).
type Reading struct {
	Start     time.Time
	End       time.Time
	KWh       float64
	Direction int // raw flowDirection code (1 or 19)
}

// ParseReadings parses a PG&E ESPI Atom feed into a slice of Readings, handling
// multiple ReadingType + IntervalBlock entries (PG&E returns import and export in
// separate runs within the same feed).
func ParseReadings(r io.Reader) ([]Reading, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var f feed
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing ESPI XML: %w", err)
	}

	// Defaults until the first ReadingType is seen: Wh→kWh (multiplier -3),
	// import direction, and energy (a feed always opens with an energy register).
	scale := 1.0 / 1000.0
	flowDir := flowDelivered
	energy := true

	var readings []Reading
	for _, e := range f.Entries {
		if rt := e.Content.ReadingType; rt != nil {
			// Energy registers report Wh (uom 72) under kind 12. A kind of 0
			// means the feed omitted it, so fall back to the unit alone.
			energy = rt.UOM == uomWh && (rt.Kind == kindEnergy || rt.Kind == 0)
			scale = math.Pow(10, float64(rt.PowerOfTenMultiplier))
			if rt.UOM == uomWh { // Wh: convert to kWh
				scale /= 1000
			}
			if rt.FlowDirection != 0 {
				flowDir = rt.FlowDirection
			}
		}

		ib := e.Content.IntervalBlock
		if ib == nil {
			continue
		}
		if !energy {
			// Non-energy channel (e.g. a demand register in watts): not kWh, so
			// skip it rather than fold watt readings into the energy series.
			continue
		}
		for _, ir := range ib.IntervalReadings {
			start := time.Unix(ir.TimePeriod.Start, 0).UTC()
			dur := time.Duration(ir.TimePeriod.Duration) * time.Second
			if dur == 0 {
				dur = 15 * time.Minute
			}
			kwh := float64(ir.Value) * scale
			if flowDir == flowReceived {
				// The received channel reports export as a positive magnitude;
				// normalise it to negative. Import readings keep their raw sign,
				// so a negative net-import interval stays a net export.
				kwh = -kwh
			}
			readings = append(readings, Reading{
				Start:     start,
				End:       start.Add(dur),
				KWh:       kwh,
				Direction: flowDir,
			})
		}
	}

	return readings, nil
}

// ExtractElectricXML ensures the Green Button export at path ends up as a plain
// XML file on disk and returns its path.
//
// If path is already an XML file it is returned unchanged.
//
// If path is a ZIP, the electric-usage XML entry is written to the same
// directory as the ZIP (using the entry's own filename), then the ZIP and every
// non-electric entry (gas XML, etc.) are deleted.  The naming heuristic mirrors
// pickElectricCSV: prefer an entry whose lowercased name contains "electric";
// fall back to the sole XML entry when only one is present.
func ExtractElectricXML(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !isZipXML(data) {
		return path, nil // already a plain XML file
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("PG&E XML export: opening zip %s: %w", path, err)
	}

	// Pick the electric XML entry.
	var xmlFiles []*zip.File
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			xmlFiles = append(xmlFiles, f)
		}
	}
	var electric *zip.File
	for _, f := range xmlFiles {
		if strings.Contains(strings.ToLower(f.Name), "electric") {
			electric = f
			break
		}
	}
	if electric == nil && len(xmlFiles) == 1 {
		electric = xmlFiles[0]
	}
	if electric == nil {
		return "", fmt.Errorf("PG&E XML export: no electric XML found in zip %s (%d XML entries)", path, len(xmlFiles))
	}

	// Write the electric XML to disk.
	dir := filepath.Dir(path)
	outPath := filepath.Join(dir, electric.Name)
	rc, err := electric.Open()
	if err != nil {
		return "", fmt.Errorf("PG&E XML export: opening %s in zip: %w", electric.Name, err)
	}
	content, readErr := io.ReadAll(rc)
	rc.Close()
	if readErr != nil {
		return "", fmt.Errorf("PG&E XML export: reading %s from zip: %w", electric.Name, readErr)
	}
	if err := os.WriteFile(outPath, content, 0o644); err != nil {
		return "", fmt.Errorf("PG&E XML export: writing %s: %w", outPath, err)
	}

	// Delete non-electric XML entries that were also in the archive.
	for _, f := range xmlFiles {
		if f == electric {
			continue
		}
		_ = os.Remove(filepath.Join(dir, f.Name))
	}
	// Delete the ZIP itself.
	_ = os.Remove(path)

	return outPath, nil
}

// ParseXMLDownload reads a Green Button XML export from path and returns the
// parsed interval readings. The file must be a plain ESPI Atom/XML (not a ZIP);
// call ExtractElectricXML first if the download may be a ZIP archive.
func ParseXMLDownload(path string) ([]Reading, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseReadings(bytes.NewReader(data))
}

// isZipXML reports whether b starts with the ZIP local-file-header magic.
func isZipXML(b []byte) bool {
	return len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && b[2] == 0x03 && b[3] == 0x04
}

// dayAccum accumulates readings for one calendar day before the final DayUsage
// is built. The intervals map is keyed by the RFC3339 start timestamp so that
// import and export readings for the same 15-minute slot are merged into a
// single IntervalRecord.
type dayAccum struct {
	importKWh float64
	exportKWh float64
	intervals map[string]*IntervalRecord // key: RFC3339 start in loc
}

// AggregateReadingsByDay groups ESPI interval readings by Pacific calendar day,
// merging the import and export channels (which arrive as separate passes in the
// XML) into one IntervalRecord per 15-minute slot. Each returned DayUsage
// carries the daily totals and the full sorted interval list.
func AggregateReadingsByDay(readings []Reading, loc *time.Location) []DayUsage {
	byDay := make(map[string]*dayAccum)
	for _, r := range readings {
		startLocal := r.Start.In(loc)
		date := startLocal.Format(dateFormat)
		acc, ok := byDay[date]
		if !ok {
			acc = &dayAccum{intervals: make(map[string]*IntervalRecord)}
			byDay[date] = acc
		}

		startKey := startLocal.Format(time.RFC3339)
		iv, ok := acc.intervals[startKey]
		if !ok {
			iv = &IntervalRecord{
				Start: startKey,
				End:   r.End.In(loc).Format(time.RFC3339),
			}
			acc.intervals[startKey] = iv
		}

		if r.Direction == flowDelivered {
			kwh := round(r.KWh, 4)
			iv.ImportKWh = round(iv.ImportKWh+kwh, 4)
			acc.importKWh += r.KWh
		} else {
			kwh := round(-r.KWh, 4) // ParseReadings negates export; restore magnitude
			iv.ExportKWh = round(iv.ExportKWh+kwh, 4)
			acc.exportKWh += -r.KWh
		}
	}

	out := make([]DayUsage, 0, len(byDay))
	for date, acc := range byDay {
		ivSlice := make([]IntervalRecord, 0, len(acc.intervals))
		for _, iv := range acc.intervals {
			ivSlice = append(ivSlice, *iv)
		}
		sort.Slice(ivSlice, func(i, j int) bool { return ivSlice[i].Start < ivSlice[j].Start })

		out = append(out, DayUsage{
			Date:      date,
			ImportKWh: round(acc.importKWh, 3),
			ExportKWh: round(acc.exportKWh, 3),
			NetKWh:    round(acc.importKWh-acc.exportKWh, 3),
			Intervals: ivSlice,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// round rounds v to the given number of decimal places.
func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
