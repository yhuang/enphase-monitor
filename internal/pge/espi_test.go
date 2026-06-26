package pge

import (
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

// TestParseReadings_MultiChannel parses a feed shaped like the GreenButtonAlliance
// sandbox export: an import register, a demand register in watts, a signed net
// register, and a received register. It pins the parser's energy-only filtering
// and signed-value handling.
func TestParseReadings_MultiChannel(t *testing.T) {
	data, err := os.ReadFile("testdata/multichannel_feed.xml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	readings, err := ParseReadings(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("ParseReadings: %v", err)
	}

	// Import (2) + net (2) + received (1) = 5; the demand register is excluded.
	if len(readings) != 5 {
		t.Fatalf("got %d readings, want 5 (demand register should be dropped)", len(readings))
	}

	// The demand reading (1500 W -> 1.5 if mis-scaled as energy) must not appear.
	for i, r := range readings {
		if r.KWh == 1.5 {
			t.Errorf("readings[%d] = 1.5 kWh: demand register leaked into the energy series", i)
		}
	}

	want := []struct {
		kwh float64
		dir int
	}{
		{0.25, flowDelivered},  // import 250 Wh
		{0.10, flowDelivered},  // import 100 Wh
		{-0.20, flowDelivered}, // net register: negative reading stays a net export
		{0.30, flowDelivered},  // net register: positive interval
		{-0.40, flowReceived},  // received 400 Wh, normalised negative
	}
	for i, w := range want {
		if readings[i].KWh != w.kwh {
			t.Errorf("readings[%d].KWh = %v, want %v", i, readings[i].KWh, w.kwh)
		}
		if readings[i].Direction != w.dir {
			t.Errorf("readings[%d].Direction = %d, want %d", i, readings[i].Direction, w.dir)
		}
	}

	// The sandbox uses 899 s intervals, not 900; the parser trusts the per-reading
	// duration rather than rounding to 15 minutes.
	if got := readings[0].End.Sub(readings[0].Start); got != 899*time.Second {
		t.Errorf("interval duration = %v, want 899s", got)
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
