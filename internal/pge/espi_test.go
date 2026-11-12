package pge

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

// A minimal two-direction ESPI feed: a ReadingType+IntervalBlock pair for import
// (flowDirection 1) followed by another pair for export (flowDirection 19). Both
// use uom 72 (Wh) and powerOfTenMultiplier 0, so a raw value of 250 Wh scales to
// 0.25 kWh. Interval starts are one 15-minute step apart.
const twoDirectionFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://naesb.org/espi">
  <entry><content>
    <ReadingType>
      <flowDirection>1</flowDirection>
      <powerOfTenMultiplier>0</powerOfTenMultiplier>
      <uom>72</uom>
      <intervalLength>900</intervalLength>
    </ReadingType>
  </content></entry>
  <entry><content>
    <IntervalBlock>
      <interval><duration>900</duration><start>1700000000</start></interval>
      <IntervalReading>
        <timePeriod><duration>900</duration><start>1700000000</start></timePeriod>
        <value>250</value>
      </IntervalReading>
    </IntervalBlock>
  </content></entry>
  <entry><content>
    <ReadingType>
      <flowDirection>19</flowDirection>
      <powerOfTenMultiplier>0</powerOfTenMultiplier>
      <uom>72</uom>
      <intervalLength>900</intervalLength>
    </ReadingType>
  </content></entry>
  <entry><content>
    <IntervalBlock>
      <interval><duration>900</duration><start>1700000900</start></interval>
      <IntervalReading>
        <timePeriod><duration>900</duration><start>1700000900</start></timePeriod>
        <value>400</value>
      </IntervalReading>
    </IntervalBlock>
  </content></entry>
</feed>`

func TestParseReadings_SignsByFlowDirection(t *testing.T) {
	readings, err := ParseReadings(strings.NewReader(twoDirectionFeed))
	if err != nil {
		t.Fatalf("ParseReadings: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("got %d readings, want 2", len(readings))
	}

	// Import: 250 Wh -> +0.25 kWh, flowDirection 1.
	if got, want := readings[0].KWh, 0.25; got != want {
		t.Errorf("import KWh = %v, want %v", got, want)
	}
	if readings[0].Direction != flowDelivered {
		t.Errorf("import Direction = %d, want %d", readings[0].Direction, flowDelivered)
	}

	// Export: 400 Wh -> -0.40 kWh (normalised negative), flowDirection 19.
	if got, want := readings[1].KWh, -0.40; got != want {
		t.Errorf("export KWh = %v, want %v", got, want)
	}
	if readings[1].Direction != flowReceived {
		t.Errorf("export Direction = %d, want %d", readings[1].Direction, flowReceived)
	}
}

// parseRealFixture loads the redacted real PG&E export for 2025-11-08.
func parseRealFixture(t *testing.T) []Reading {
	t.Helper()
	data, err := os.ReadFile("testdata/pge_real_2025-11-08.xml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	readings, err := ParseReadings(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("ParseReadings: %v", err)
	}
	return readings
}

// TestParseReadings_RealFeedUsesGrossAndReceivedOnly parses a real PG&E export
// (four registers: signed net, received, demand, gross delivered) and checks the
// day totals against the known answer. Summing the two flowDirection-1 registers
// would give ~33.8 kWh of import (gross + net) instead of 29.724.
func TestParseReadings_RealFeedUsesGrossAndReceivedOnly(t *testing.T) {
	readings := parseRealFixture(t)

	// 96 gross + 96 received; the net and demand registers must not appear.
	if len(readings) != 192 {
		t.Fatalf("got %d readings, want 192 (gross + received only)", len(readings))
	}
	var imp, exp float64
	for _, r := range readings {
		switch r.Direction {
		case flowDelivered:
			if r.KWh < 0 {
				t.Fatalf("negative delivered reading %v at %v: net register leaked in", r.KWh, r.Start)
			}
			imp += r.KWh
		case flowReceived:
			exp += -r.KWh
		}
	}
	if math.Abs(imp-29.724) > 1e-6 {
		t.Errorf("import = %.4f kWh, want 29.724", imp)
	}
	if math.Abs(exp-25.675) > 1e-6 {
		t.Errorf("export = %.4f kWh, want 25.675", exp)
	}
}

// TestAggregate_RealFeedLocalClockAndTotals checks the daily aggregate and the
// clock decoding: PG&E's "UTC" stamps are local wall-clock, so 2025-11-08 starts
// at 00:00-08:00 and solar export falls between 09:00 and 17:00 local.
func TestAggregate_RealFeedLocalClockAndTotals(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	days, err := AggregateReadingsByDay(parseRealFixture(t), loc)
	if err != nil {
		t.Fatalf("AggregateReadingsByDay: %v", err)
	}
	if len(days) != 1 || days[0].Date != "2025-11-08" {
		t.Fatalf("days = %+v, want one day 2025-11-08", days)
	}
	d := days[0]
	if d.ImportKWh != 29.724 || d.ExportKWh != 25.675 || d.NetKWh != 4.049 {
		t.Errorf("totals import/export/net = %v/%v/%v, want 29.724/25.675/4.049", d.ImportKWh, d.ExportKWh, d.NetKWh)
	}
	if len(d.Intervals) != 96 || d.Intervals[0].Start != "2025-11-08T00:00:00-08:00" {
		t.Fatalf("intervals = %d, first %q; want 96 starting 2025-11-08T00:00:00-08:00", len(d.Intervals), d.Intervals[0].Start)
	}
	for _, iv := range d.Intervals {
		if iv.ImportKWh < 0 {
			t.Errorf("%s: negative import %v", iv.Start, iv.ImportKWh)
		}
		if iv.ExportKWh > 0.1 {
			st, _ := time.Parse(time.RFC3339, iv.Start)
			if h := st.Hour(); h < 9 || h >= 17 {
				t.Errorf("%s: solar export %v kWh outside 09:00-17:00 local", iv.Start, iv.ExportKWh)
			}
		}
	}
}

// reg builds a register of 1 kWh-scaled readings on 15-minute slots from a
// midnight label.
func reg(flow int, wh ...int64) register {
	r := register{flow: flow}
	base := time.Date(2025, 11, 8, 0, 0, 0, 0, time.UTC)
	for i, v := range wh {
		s := base.Add(time.Duration(i) * 15 * time.Minute)
		r.readings = append(r.readings, Reading{Start: s, End: s.Add(15 * time.Minute), KWh: float64(v) / 1000, Direction: flow})
	}
	return r
}

func TestSelectRegisters_NetAndGrossDoNotDoubleCount(t *testing.T) {
	gross := reg(flowDelivered, 500, 300, 0, 100)
	recv := reg(flowReceived, 0, 0, 200, 400)
	net := reg(flowDelivered, 500, 300, -200, -300) // gross − received

	// Either file order must pick the same gross register.
	for name, regs := range map[string][]register{
		"net first":   {net, recv, gross},
		"gross first": {gross, recv, net},
	} {
		got, err := selectRegisters(regs)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var imp, exp float64
		for _, r := range got {
			if r.Direction == flowDelivered {
				imp += r.KWh
			} else {
				exp -= r.KWh
			}
		}
		if math.Abs(imp-0.9) > 1e-9 || math.Abs(exp-0.6) > 1e-9 {
			t.Errorf("%s: import %.3f export %.3f, want 0.900 / 0.600", name, imp, exp)
		}
	}
}

func TestSelectRegisters_Errors(t *testing.T) {
	gross := reg(flowDelivered, 500, 300)
	recv := reg(flowReceived, 0, 0)
	if _, err := selectRegisters([]register{gross, reg(flowDelivered, 1, 2), recv}); err == nil {
		t.Error("two delivered registers that do not satisfy net = delivered - received: want error")
	}
	if _, err := selectRegisters([]register{reg(flowDelivered, 500, -300)}); err == nil {
		t.Error("lone delivered register with negative values: want error")
	}
	if _, err := selectRegisters([]register{gross, gross, gross}); err == nil {
		t.Error("three delivered registers: want error")
	}
}

func TestAggregate_UndetectableClockEncodingFails(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	// 1700000000 is 22:13:20Z: a midnight in neither UTC nor Pacific.
	start := time.Unix(1700000000, 0).UTC()
	_, err = AggregateReadingsByDay([]Reading{{Start: start, End: start.Add(15 * time.Minute), KWh: 1, Direction: flowDelivered}}, loc)
	if err == nil {
		t.Fatal("want error for timestamps aligned to neither UTC nor local midnight")
	}
}

func TestAggregate_TrueUTCEncodingIsAccepted(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	start := time.Date(2025, 11, 8, 8, 0, 0, 0, time.UTC) // local midnight as a real instant
	days, err := AggregateReadingsByDay([]Reading{{Start: start, End: start.Add(15 * time.Minute), KWh: 1, Direction: flowDelivered}}, loc)
	if err != nil || len(days) != 1 || days[0].Intervals[0].Start != "2025-11-08T00:00:00-08:00" {
		t.Fatalf("days = %+v, err = %v; want a true-UTC feed kept at local midnight", days, err)
	}
}

func TestAggregate_SpringForwardGapLabelDoesNotMerge(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	label := func(h, m int) time.Time { return time.Date(2026, 3, 8, h, m, 0, 0, time.UTC) }
	var rs []Reading
	// PG&E on 2026-03-08: ... 01:45, then 02:00 (a label inside the gap), then 03:15.
	for _, l := range []time.Time{label(0, 0), label(1, 45), label(2, 0), label(3, 15)} {
		rs = append(rs, Reading{Start: l, End: l.Add(15 * time.Minute), KWh: 1, Direction: flowDelivered})
	}
	days, err := AggregateReadingsByDay(rs, loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(days[0].Intervals); got != 4 || days[0].ImportKWh != 4 {
		t.Fatalf("intervals = %d, import = %v; want 4 distinct intervals totalling 4 (the 02:00 label must not fold onto 01:00)", got, days[0].ImportKWh)
	}
	if got := days[0].Intervals[2].Start; got != "2026-03-08T03:00:00-07:00" {
		t.Errorf("gap label mapped to %s, want 2026-03-08T03:00:00-07:00", got)
	}
}

func TestCheckDays(t *testing.T) {
	loc := time.UTC
	from := time.Date(2025, 11, 8, 0, 0, 0, 0, loc)
	bad := DayUsage{Date: "2025-11-08", ImportKWh: -1, Intervals: []IntervalRecord{{ImportKWh: -0.1, ExportKWh: 0.2}}}
	if got := CheckDays([]DayUsage{bad}, "", from, from, loc); len(got) != 3 {
		t.Errorf("warnings = %v, want negative import, mixed interval, interval count", got)
	}
}

func TestParseReadings_Empty(t *testing.T) {
	readings, err := ParseReadings(strings.NewReader(`<feed xmlns="http://naesb.org/espi"></feed>`))
	if err != nil {
		t.Fatalf("ParseReadings: %v", err)
	}
	if len(readings) != 0 {
		t.Fatalf("got %d readings, want 0", len(readings))
	}
}

// aggregateFixture parses a testdata feed and aggregates it in Pacific time.
func aggregateFixture(t *testing.T, file string) DayUsage {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	readings, err := ParseReadings(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("ParseReadings: %v", err)
	}
	days, err := AggregateReadingsByDay(readings, loc)
	if err != nil || len(days) != 1 {
		t.Fatalf("AggregateReadingsByDay = %v, %v; want one day", days, err)
	}
	for _, iv := range days[0].Intervals {
		if iv.ImportKWh < 0 {
			t.Errorf("%s: negative import %v", iv.Start, iv.ImportKWh)
		}
	}
	return days[0]
}

// TestAggregate_RealSpringForwardDay pins the real 2026-03-08 feed: 92 intervals
// is the correct length of that 23-hour local day (not a symptom), the local hour
// 02:00-03:00 does not exist, and PG&E's gap-labelled slot lands on 03:00 PDT.
func TestAggregate_RealSpringForwardDay(t *testing.T) {
	d := aggregateFixture(t, "pge_real_2026-03-08_spring-forward.xml")
	if d.Date != "2026-03-08" || d.ImportKWh != 16.844 || d.ExportKWh != 47.816 || d.NetKWh != -30.972 {
		t.Errorf("day = %s import %v export %v net %v; want 2026-03-08 16.844 / 47.816 / -30.972", d.Date, d.ImportKWh, d.ExportKWh, d.NetKWh)
	}
	if len(d.Intervals) != 92 {
		t.Fatalf("intervals = %d, want 92", len(d.Intervals))
	}
	if first, last := d.Intervals[0].Start, d.Intervals[91].Start; first != "2026-03-08T00:00:00-08:00" || last != "2026-03-08T23:45:00-07:00" {
		t.Errorf("first/last start = %s / %s", first, last)
	}
	var has0300 bool
	for _, iv := range d.Intervals {
		if strings.HasPrefix(iv.Start, "2026-03-08T02:") {
			t.Errorf("interval %s lies in the nonexistent 02:00 hour", iv.Start)
		}
		has0300 = has0300 || iv.Start == "2026-03-08T03:00:00-07:00"
	}
	if !has0300 {
		t.Error("the slot PG&E labels 02:00 should map to 03:00-07:00")
	}
	la := mustLA(t)
	day := time.Date(2026, 3, 8, 0, 0, 0, 0, la)
	if w := CheckDays([]DayUsage{d}, "", day, day, la); len(w) != 0 {
		t.Errorf("a 92-interval spring-forward day should not warn, got %v", w)
	}
}

// TestAggregate_RealFallBackDay pins the real 2025-11-02 feed: PG&E supplies only
// 96 of the day's 100 real slots (the repeated hour is absent from the source file
// itself), so the day is understated and CheckDays must say so.
func TestAggregate_RealFallBackDay(t *testing.T) {
	d := aggregateFixture(t, "pge_real_2025-11-02_fall-back.xml")
	if d.Date != "2025-11-02" || d.ImportKWh != 22.738 || d.ExportKWh != 6.123 {
		t.Errorf("day = %s import %v export %v; want 2025-11-02 22.738 / 6.123", d.Date, d.ImportKWh, d.ExportKWh)
	}
	if len(d.Intervals) != 96 {
		t.Fatalf("intervals = %d, want 96", len(d.Intervals))
	}
	la := mustLA(t)
	day := time.Date(2025, 11, 2, 0, 0, 0, 0, la)
	w := CheckDays([]DayUsage{d}, "", day, day, la)
	if len(w) != 1 || !strings.Contains(w[0], "96 intervals, expected 100") {
		t.Errorf("warnings = %v, want one saying 96 intervals, expected 100", w)
	}
}

func mustLA(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	return loc
}

func TestExpectedIntervals(t *testing.T) {
	loc := mustLA(t)
	for date, want := range map[string]int{"2025-11-08": 96, "2026-03-08": 92, "2025-11-02": 100} {
		if got := expectedIntervals(date, loc); got != want {
			t.Errorf("expectedIntervals(%s) = %d, want %d", date, got, want)
		}
	}
}
