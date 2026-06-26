package enphase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"enphase-monitor/internal/browser"

	"github.com/chromedp/chromedp"
)

// Timing constants for the portal stats UI. The portal updates totals a beat
// after the date range or selected application changes, so each interaction is
// followed by a short settle, and the hit total is read until it stops moving.
const (
	hitsStableReads      = 2                       // identical reads before a total is "settled"
	hitsPollTimeout      = 30 * time.Second        // overall wait for the total to settle
	hitsPollInterval     = 750 * time.Millisecond  // delay between hit-total reads
	appSelectSettle      = 800 * time.Millisecond  // re-render after selecting an application
	appSelectRetrySettle = 500 * time.Millisecond  // dropdown-open delay before retrying selection
	calendarOpenSettle   = 600 * time.Millisecond  // calendar popup render delay after clicking the input
	dateRangeSettle      = 1200 * time.Millisecond // results reload after the date selection is complete
	readyPollInterval    = 2 * time.Second         // delay between stats-page readiness checks
)

// StatsScraper drives a headed Chrome session on the developer portal stats page.
// It holds the chromedp session contexts (parent governs the session lifetime,
// ctx is the active browser context) — storing them is the accepted pattern for
// a session wrapper, since chromedp itself ties a session to a context.
type StatsScraper struct {
	parent  context.Context
	ctx     context.Context
	cancel  func()
	started bool
}

// NewStatsScraper returns a scraper whose Chrome session is governed by parent.
func NewStatsScraper(parent context.Context) *StatsScraper {
	return &StatsScraper{parent: parent}
}

// Close shuts down the Chrome session.
func (s *StatsScraper) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *StatsScraper) ensureStarted() error {
	if s.started {
		return nil
	}
	ctx, cancel, err := browser.LaunchHeaded(s.parent)
	if err != nil {
		return err
	}
	s.ctx, s.cancel, s.started = ctx, cancel, true
	return nil
}

func (s *StatsScraper) openStatsPage() error {
	if err := s.ensureStarted(); err != nil {
		return err
	}
	return chromedp.Run(s.ctx, chromedp.Navigate(statsPageURL))
}

func (s *StatsScraper) waitForStatsReady() error {
	deadline := time.Now().Add(statsWaitTimeout)
	for {
		if s.parent.Err() != nil {
			return s.parent.Err()
		}
		ready, err := s.statsPageReady()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for the stats page (log in at %s)", statsWaitTimeout, statsPageURL)
		}
		select {
		case <-s.parent.Done():
			return s.parent.Err()
		case <-time.After(readyPollInterval):
		}
	}
}

func (s *StatsScraper) statsPageReady() (bool, error) {
	var ready bool
	err := chromedp.Run(s.ctx, chromedp.Evaluate(`(() => {
		const text = (document.body && document.body.innerText) || '';
		return /Hits\s*\(\s*hits\s*\)/i.test(text) || /show last/i.test(text);
	})()`, &ready))
	return ready, err
}

func (s *StatsScraper) readAppMonthlyHits(appName, fromDate string) (int, error) {
	if err := s.selectApp(appName); err != nil {
		return 0, err
	}
	if err := s.setFromDate(fromDate); err != nil {
		return 0, err
	}
	return s.waitForHits()
}

func (s *StatsScraper) selectApp(appName string) error {
	nameJSON, err := json.Marshal(appName)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`(function(name) {
		const norm = s => (s || '').replace(/\s+/g, ' ').trim();
		const target = norm(name);

		for (const sel of document.querySelectorAll('select')) {
			for (const opt of sel.options) {
				if (norm(opt.textContent) === target) {
					if (sel.value !== opt.value) {
						sel.value = opt.value;
						sel.dispatchEvent(new Event('change', { bubbles: true }));
					}
					return true;
				}
			}
		}

		const clickMatch = () => {
			const opts = document.querySelectorAll('li, [role="option"], .ant-select-item, .Select-option, div, span');
			for (const el of opts) {
				if (norm(el.textContent) !== target) continue;
				const r = el.getBoundingClientRect();
				if (r.width <= 0 || r.height <= 0) continue;
				el.click();
				return true;
			}
			return false;
		};

		if (clickMatch()) return true;

		const triggers = document.querySelectorAll('[role="combobox"], .ant-select-selector, .Select-control, select + div, button');
		for (const tr of triggers) {
			const t = norm(tr.textContent);
			if (t.includes('enphase-monitor') || tr.getAttribute('role') === 'combobox') {
				tr.click();
				break;
			}
		}
		return clickMatch();
	})(%s)`, string(nameJSON))

	var ok bool
	if err := chromedp.Run(s.ctx,
		chromedp.Evaluate(script, &ok),
		chromedp.Sleep(appSelectSettle),
	); err != nil {
		return fmt.Errorf("failed to select application: %w", err)
	}
	if !ok {
		// Retry once after opening any visible enphase-monitor label.
		openScript := `(function() {
			const re = /enphase-monitor-\d+/;
			for (const el of document.querySelectorAll('div, span, button')) {
				const t = (el.textContent || '').trim();
				if (!re.test(t)) continue;
				const r = el.getBoundingClientRect();
				if (r.width <= 0 || r.height <= 0) continue;
				el.click();
				return true;
			}
			return false;
		})()`
		_ = chromedp.Run(s.ctx, chromedp.Evaluate(openScript, &ok), chromedp.Sleep(appSelectRetrySettle))
		if err := chromedp.Run(s.ctx, chromedp.Evaluate(script, &ok)); err != nil {
			return fmt.Errorf("failed to select application: %w", err)
		}
	}
	if !ok {
		return fmt.Errorf("could not find %q in the stats application dropdown — the portal UI may have changed", appName)
	}
	return nil
}

// setFromDate selects the first of the current month via the portal's jQuery UI
// datepicker and leaves the UNTIL date at its default (today).
//
// Portal HTML structure (StatsMenu-custom):
//
//	<span> from
//	  <input class="StatsMenu-customInput hasDatepicker" type="text">  ← zero-width backing input
//	  <a class="StatsMenu-customLink--since">06/01/2026</a>            ← visible click trigger
//	</span>
//	<span> until
//	  <input class="StatsMenu-customInput hasDatepicker" type="text">
//	  <a class="StatsMenu-customLink--until">06/25/2026</a>
//	</span>
//
// Sequence:
//  1. Click <a class="StatsMenu-customLink--since"> to open the jQuery UI datepicker.
//  2. Click day "1" in #ui-datepicker-div (skipping other-month cells).
//  3. Leave UNTIL alone; it already shows today.
func (s *StatsScraper) setFromDate(fromDate string) error {
	// Step 1: click the FROM date link to open the jQuery UI datepicker popup.
	// The backing <input class="hasDatepicker"> is zero-width; the visible trigger
	// is the <a class="StatsMenu-customLink--since"> anchor. Fall back to the input
	// if the anchor is somehow absent.
	openScript := `(function() {
		const link = document.querySelector('a.StatsMenu-customLink--since');
		if (link) { link.click(); return true; }
		const inp = document.querySelector('input.StatsMenu-customInput');
		if (inp) { inp.click(); inp.focus(); return true; }
		return false;
	})()`

	var ok bool
	if err := chromedp.Run(s.ctx,
		chromedp.Evaluate(openScript, &ok),
		chromedp.Sleep(calendarOpenSettle),
	); err != nil {
		return fmt.Errorf("failed to open from-date datepicker: %w", err)
	}
	if !ok {
		return errors.New("could not find from date link on the stats page — the portal UI may have changed")
	}

	// Step 2: click day "1" in the jQuery UI datepicker calendar.
	// #ui-datepicker-div contains <td> elements; days from adjacent months carry
	// class "ui-datepicker-other-month". The clickable day is an <a> inside the td.
	clickDay1Script := `(function() {
		const picker = document.querySelector('#ui-datepicker-div, .ui-datepicker');
		if (!picker) return false;
		for (const td of picker.querySelectorAll('td')) {
			if (td.classList.contains('ui-datepicker-other-month') ||
				td.classList.contains('ui-datepicker-unselectable') ||
				td.classList.contains('ui-state-disabled')) continue;
			const a = td.querySelector('a');
			if (a && a.textContent.trim() === '1') { a.click(); return true; }
		}
		return false;
	})()`

	if err := chromedp.Run(s.ctx,
		chromedp.Evaluate(clickDay1Script, &ok),
		chromedp.Sleep(dateRangeSettle),
	); err != nil {
		return fmt.Errorf("failed to click day 1 in datepicker: %w", err)
	}
	if !ok {
		return fmt.Errorf("could not find day 1 in the datepicker for %s — the portal UI may have changed", fromDate)
	}
	return nil
}

func (s *StatsScraper) waitForHits() (int, error) {
	deadline := time.Now().Add(hitsPollTimeout)
	var last int
	stable := 0
	for {
		text, err := s.pageText()
		if err != nil {
			return 0, err
		}
		if hits, ok := parseHitsTotal(text); ok {
			if hits == last {
				stable++
				if stable >= hitsStableReads {
					return hits, nil
				}
			} else {
				last = hits
				stable = 0
			}
		}
		if time.Now().After(deadline) {
			if last > 0 || strings.Contains(strings.ToLower(text), "hits") {
				return last, nil
			}
			return 0, errors.New("timed out waiting for hit total on stats page")
		}
		select {
		case <-s.parent.Done():
			return 0, s.parent.Err()
		case <-time.After(hitsPollInterval):
		}
	}
}

func (s *StatsScraper) pageText() (string, error) {
	var text string
	err := chromedp.Run(s.ctx, chromedp.Evaluate(`(document.body && document.body.innerText) || ""`, &text))
	return text, err
}
