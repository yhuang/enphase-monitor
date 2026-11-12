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
// Register layout (verified against real PG&E Green Button exports, 2025-26): a
// feed carries FOUR metered registers per electric meter, each a ReadingType
// followed by its IntervalBlock:
//
//	flowDirection 1,  uom 72 — signed NET  (delivered − received; negative while exporting)
//	flowDirection 19, uom 72 — RECEIVED    (export, positive magnitude)
//	flowDirection 0,  uom 38 — demand in W (not energy; skipped)
//	flowDirection 1,  uom 72 — GROSS DELIVERED (import, never negative)
//
// Two registers share flowDirection 1 and carry identical ReadingType attributes
// and opaque UUIDs, so metadata cannot tell the net register from the gross one
// (and file order is not a contract). selectRegisters therefore identifies them by
// the ESPI identity net = delivered − received, checked on every interval, and
// fails loudly if the feed does not satisfy it. Summing both flowDirection-1
// registers would double import while importing and
// turn it negative while exporting. Only gross delivered and received are used;
// the net register is never added.
//
// Scale: PG&E's ReadingTypes declare powerOfTenMultiplier 3 on registers whose
// values are plain Wh (uom 72) and W (uom 38): taking the multiplier literally
// makes a 15-minute interval hundreds of kWh. Daily sums of value/1000 match the
// Enphase grid meters to within a few percent on every day checked, so the
// multiplier is ignored and uom 72 values are always Wh. A magnitude guard
// (maxIntervalKWh) fails the parse if that ever stops being plausible.
//
// Energy-only: the demand register (watts, kind 8) is not kWh and is skipped.
//
// Clock encoding: PG&E stamps each interval with local wall-clock time encoded as
// if it were UTC (the feed's LocalTimeParameters say tzOffset 0 / dstOffset 0, and
// the first reading of a day-range export is 00:00 "UTC" = local midnight; solar
// export peaks at 12:00 "UTC"). Reading.Start therefore holds a clock label, not
// an instant; AggregateReadingsByDay converts it to a real local time with
// time.Date. The encoding is detected from the data, not assumed: the earliest
// reading must sit exactly on a midnight, either in UTC (wall-clock encoding) or
// in the report timezone (true-UTC encoding); anything else is an error. In
// the wall-clock encoding a spring-forward day has 92 intervals (the skipped
// local hour is absent) and a fall-back day only 96 (PG&E drops one hour), so a
// day with other than 96 intervals is reported by CheckDays.
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
	PowerOfTenMultiplier int `xml:"powerOfTenMultiplier"` // parsed but unused; see "Scale" in the file header
	UOM                  int `xml:"uom"`                  // 72 = Wh, 38 = W
	Kind                 int `xml:"kind"`                 // 12 = energy, 8 = demand
	IntervalLength       int `xml:"intervalLength"`       // seconds; ~900 = 15 min
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
	Start     time.Time // clock label as stamped by PG&E; see "Clock encoding" in the file header
	End       time.Time
	KWh       float64
	Direction int // raw flowDirection code (1 or 19)
}

// register is one energy ReadingType with its readings, before selection.
type register struct {
	flow     int
	readings []Reading // KWh unsigned (raw sign preserved), Direction = flow
}

// ParseReadings parses a PG&E ESPI Atom feed into readings for the two registers
// that matter: gross delivered (import, positive) and received (export, negative).
// See "Register layout" in the file header for how the net register is excluded.
// Reading.Start is a clock label, not an instant (see "Clock encoding").
func ParseReadings(r io.Reader) ([]Reading, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var f feed
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing ESPI XML: %w", err)
	}

	// Defaults until the first ReadingType is seen: import direction and energy (a
	// feed always opens with an energy register). Values are Wh; see "Scale".
	const scale = 1.0 / 1000.0
	flowDir := flowDelivered
	energy := true

	var regs []register
	for _, e := range f.Entries {
		if rt := e.Content.ReadingType; rt != nil {
			// Energy registers report Wh (uom 72) under kind 12. A kind of 0
			// means the feed omitted it, so fall back to the unit alone.
			energy = rt.UOM == uomWh && (rt.Kind == kindEnergy || rt.Kind == 0)
			if rt.FlowDirection != 0 {
				flowDir = rt.FlowDirection
			}
		}

		ib := e.Content.IntervalBlock
		if ib == nil || !energy {
			continue // non-energy channel (e.g. demand in watts) is not kWh
		}
		reg := register{flow: flowDir}
		for _, ir := range ib.IntervalReadings {
			start := time.Unix(ir.TimePeriod.Start, 0).UTC()
			dur := time.Duration(ir.TimePeriod.Duration) * time.Second
			if dur == 0 {
				dur = 15 * time.Minute
			}
			if kwh := math.Abs(float64(ir.Value) * scale); kwh > maxIntervalKWh {
				return nil, fmt.Errorf("PG&E ESPI feed: implausible %.1f kWh in one interval at %s (flowDirection %d); the value scale is probably wrong",
					kwh, start.Format(time.RFC3339), flowDir)
			}
			reg.readings = append(reg.readings, Reading{
				Start:     start,
				End:       start.Add(dur),
				KWh:       float64(ir.Value) * scale,
				Direction: flowDir,
			})
		}
		regs = append(regs, reg)
	}

	return selectRegisters(regs)
}

// maxIntervalKWh bounds one 15-minute reading (25 kWh = 100 kW sustained), far
// above any real service but far below a wrongly scaled value.
const maxIntervalKWh = 25

// netTolerance is the largest per-interval disagreement (kWh) accepted when
// checking net = delivered − received; values are whole Wh, so 1.5 Wh covers
// rounding without admitting a wrong register.
const netTolerance = 0.0015

// selectRegisters picks gross delivered and received from the energy registers
// and returns their readings, received negated. It errors when the registers do
// not match the layout described in the file header.
func selectRegisters(regs []register) ([]Reading, error) {
	var delivered []register
	var received *register
	for i := range regs {
		switch regs[i].flow {
		case flowDelivered:
			delivered = append(delivered, regs[i])
		case flowReceived:
			if received != nil {
				return nil, fmt.Errorf("PG&E ESPI feed: more than one received (flowDirection %d) energy register", flowReceived)
			}
			received = &regs[i]
		}
	}

	recv := map[time.Time]float64{}
	if received != nil {
		for _, r := range received.readings {
			recv[r.Start] = r.KWh
		}
	}

	var gross *register
	switch len(delivered) {
	case 0:
	case 1:
		for _, r := range delivered[0].readings {
			if r.KWh < 0 {
				return nil, fmt.Errorf("PG&E ESPI feed: the only delivered register has negative readings (looks like a signed net register); cannot derive gross import")
			}
		}
		gross = &delivered[0]
	case 2:
		// Gross is the register g such that g − received == the other, on every slot.
		ok := [2]bool{matchesNet(delivered[0], delivered[1], recv), matchesNet(delivered[1], delivered[0], recv)}
		switch {
		case ok[0]:
			gross = &delivered[0] // when both hold the registers are identical, so either is right
		case ok[1]:
			gross = &delivered[1]
		default:
			return nil, fmt.Errorf("PG&E ESPI feed: two delivered registers but neither satisfies net = delivered − received; refusing to guess which is gross import")
		}
	default:
		return nil, fmt.Errorf("PG&E ESPI feed: %d delivered (flowDirection %d) energy registers; expected 1 or 2", len(delivered), flowDelivered)
	}

	var out []Reading
	if gross != nil {
		out = append(out, gross.readings...)
	}
	if received != nil {
		for _, r := range received.readings {
			r.KWh = -r.KWh // received reports export as a positive magnitude
			out = append(out, r)
		}
	}
	return out, nil
}

// matchesNet reports whether gross − received equals net on every slot of both
// registers (a slot absent from received counts as 0).
func matchesNet(gross, net register, recv map[time.Time]float64) bool {
	if len(gross.readings) != len(net.readings) {
		return false
	}
	netBy := make(map[time.Time]float64, len(net.readings))
	for _, r := range net.readings {
		netBy[r.Start] = r.KWh
	}
	for _, g := range gross.readings {
		n, ok := netBy[g.Start]
		if !ok || math.Abs(g.KWh-recv[g.Start]-n) > netTolerance {
			return false
		}
	}
	return true
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

// AggregateReadingsByDay groups ESPI interval readings by local calendar day,
// merging the import and export channels (which arrive as separate passes in the
// XML) into one IntervalRecord per 15-minute slot. Each returned DayUsage
// carries the daily totals and the full sorted interval list.
func AggregateReadingsByDay(readings []Reading, loc *time.Location) ([]DayUsage, error) {
	wallClock, err := detectWallClock(readings, loc)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]*dayAccum)
	for _, r := range readings {
		startLocal := localStart(r.Start, wallClock, loc)
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
				End:   startLocal.Add(r.End.Sub(r.Start)).Format(time.RFC3339),
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
	return out, nil
}

// detectWallClock decides how the feed encodes its timestamps from the earliest
// reading, which must sit on a midnight: in UTC (PG&E's local-wall-clock-as-UTC
// encoding, returns true) or in loc (true UTC, returns false). Any other
// alignment means the encoding is unknown, so it errors instead of guessing.
func detectWallClock(readings []Reading, loc *time.Location) (bool, error) {
	if len(readings) == 0 {
		return false, nil
	}
	first := readings[0].Start
	for _, r := range readings {
		if r.Start.Before(first) {
			first = r.Start
		}
	}
	isMidnight := func(t time.Time) bool { return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 }
	switch {
	case isMidnight(first.UTC()):
		return true, nil
	case isMidnight(first.In(loc)):
		return false, nil
	}
	return false, fmt.Errorf("PG&E feed timestamps: earliest reading %s is on a midnight in neither UTC nor %s; cannot tell whether timestamps are local wall-clock or true UTC",
		first.UTC().Format(time.RFC3339), loc)
}

// localStart converts a reading's Start to a real instant in loc. With the
// wall-clock encoding the UTC clock fields are the local clock, so they are
// reinterpreted in loc (this follows DST); otherwise Start is already an instant.
func localStart(t time.Time, wallClock bool, loc *time.Location) time.Time {
	if !wallClock {
		return t.In(loc)
	}
	u := t.UTC()
	local := time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), 0, loc)
	if local.Hour() != u.Hour() || local.Minute() != u.Minute() {
		// The label falls in a spring-forward gap (e.g. 02:00 on the transition
		// day) and Go folded it onto an earlier slot, which would merge two
		// intervals. PG&E labels the first interval after the gap this way, so
		// map it to the same clock minute one hour later (03:00).
		local = time.Date(u.Year(), u.Month(), u.Day(), u.Hour()+1, u.Minute(), u.Second(), 0, loc)
	}
	return local
}

// round rounds v to the given number of decimal places.
func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
