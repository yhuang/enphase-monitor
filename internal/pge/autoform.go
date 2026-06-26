// autoform.go auto-drives PG&E's Green Button export form.
//
// The Green Button lives inside a Visualforce iframe
// (myaccount.pge.com/myaccount/apex/myAcct_VF_GreenButton) that is embedded in
// PG&E's Salesforce Lightning page. The Lightning outer page uses closed shadow
// roots, but the Visualforce iframe itself is plain HTML with no shadow DOM —
// standard querySelector works fine inside it.
//
// Strategy:
//  1. Find the iframe's frame ID by URL pattern from the CDP frame tree.
//  2. Get the iframe's screen position via DOM.getFrameOwner (CDP-level, bypasses
//     shadow DOM) + DOM.getBoxModel.
//  3. Create an isolated JS world in the iframe with Page.createIsolatedWorld,
//     which returns an ExecutionContextID immediately without event listening.
//  4. Evaluate getBoundingClientRect() in that context to find each element's
//     position, then dispatch mouse clicks at (iframeOrigin + elementPos).
//  5. Set date values via JS (value setter + input/change/blur events).
//
// Entry point: formDriver.drive.
package pge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	cdpdom "github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	cdpnetwork "github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// exportFormTimeout bounds the automated form-driving attempt, including page
// navigation, OneTrust dismissal, chart load, and form fill.
const exportFormTimeout = 3 * time.Minute

// oneTrustDismissScript is injected into every new document's MAIN JS world via
// Page.addScriptToEvaluateOnNewDocument.
//
// Strategy: call window.OneTrust.AllowAll() FIRST so that Opower and other
// widgets receive the proper consent callback and initialise correctly. Only
// AFTER AllowAll has fired (500 ms later) do we remove the remaining overlay
// DOM elements. Removing elements without firing AllowAll causes downstream
// widgets to stay in an unconsented/blank state.
const oneTrustDismissScript = `(function(){
	function removeOT(){
		['onetrust-consent-sdk','onetrust-banner-sdk','onetrust-pc-sdk',
		 'onetrust-reject-all-handler-container'].forEach(function(id){
			var el=document.getElementById(id);
			if(el&&el.parentNode){el.parentNode.removeChild(el);}
		});
		document.querySelectorAll('.onetrust-pc-dark-filter,.onetrust-overlay')
			.forEach(function(el){if(el.parentNode){el.parentNode.removeChild(el);}});
		if(document.body){document.body.style.overflow='';document.body.style.position='';}
	}

	// Poll for window.OneTrust and call AllowAll() the moment it is available.
	// AllowAll fires consent callbacks that downstream widgets listen for; only
	// then do we remove the overlay so we don't race those callbacks.
	var _allowAllDone=false;
	var _poll=setInterval(function(){
		if(window.OneTrust&&typeof window.OneTrust.AllowAll==='function'&&!_allowAllDone){
			_allowAllDone=true;
			clearInterval(_poll);
			try{window.OneTrust.AllowAll();}catch(e){}
			// Remove overlay 500 ms after AllowAll so callbacks have time to fire.
			setTimeout(removeOT,500);
		}
	},50);
	// Give up polling after 15 s and just remove elements as a last resort.
	setTimeout(function(){
		clearInterval(_poll);
		if(!_allowAllDone){removeOT();}
	},15000);

	// Pre-set consent cookies so OneTrust may skip the banner on future loads.
	try{
		var d=new Date().toISOString();
		var host=location.hostname;
		document.cookie='OptanonAlertBoxClosed='+d+';path=/;domain='+host;
		document.cookie='OptanonConsent=isGpcEnabled%3D0%26groups%3DC0001%3A1%2CC0002%3A1%2CC0003%3A1%2CC0004%3A1;path=/;domain='+host;
	}catch(e){}
})();`

// sfScrollScript scrolls every plausible Salesforce Lightning scroll container.
// Salesforce LWC renders content inside a <main> or .siteBody element rather
// than relying on window scroll, so window.scrollBy alone never reaches the
// intersection observer that lazy-loads the Green Button VF iframe.
const sfScrollScript = `(function(){
	var h = window.innerHeight;
	['main','[role="main"]','.siteBody','.forceSiteBody',
	 '.contentArea','.content-area','.slds-template__container',
	 'section','article'].forEach(function(sel){
		document.querySelectorAll(sel).forEach(function(el){
			el.scrollTop += h;
		});
	});
	window.scrollBy(0, h);
	try { document.documentElement.scrollTop += h; } catch(e) {}
	try { document.body.scrollTop += h; } catch(e) {}
})()`

// deepQueryJS is a JS snippet that defines deepQuery(root, sel), which finds the
// first element matching sel inside root, piercing open shadow roots. Embed this
// in any evalInFrame expression that needs to locate elements that may live inside
// Opower's Web Component shadow trees.
const deepQueryJS = `function deepQuery(root,sel){
	var el=root.querySelector(sel);
	if(el) return el;
	var nodes=root.querySelectorAll('*');
	for(var i=0;i<nodes.length;i++){
		if(nodes[i].shadowRoot){var f=deepQuery(nodes[i].shadowRoot,sel);if(f)return f;}
	}
	return null;
}`

// ---- formDriver ----------------------------------------------------------------

// formDriver drives a single Green Button form automation run. All diagnostic
// output is buffered internally so that noisy step-by-step logging is suppressed
// on successful runs; callers flush the buffer to stderr only when the auto-drive
// fails.
type formDriver struct {
	buf    bytes.Buffer
	notify func(string) // optional status sink, may be nil
}

// debug appends a formatted diagnostic line to the internal buffer.
func (fd *formDriver) debug(format string, args ...any) {
	fmt.Fprintf(&fd.buf, "[pge/form] "+format+"\n", args...)
}

// flushTo writes all buffered diagnostics to w and resets the buffer. Callers
// invoke this after a failed drive() to surface the diagnostic trail.
func (fd *formDriver) flushTo(w io.Writer) {
	_, _ = fd.buf.WriteTo(w)
}

// drive fills in PG&E's Green Button export form for the date range [from, to].
//
// Confirmed page structure (from live frame-tree dumps):
//
//	GreenButton VF iframe (myAcct_VF_GreenButton):
//	  - Loads OneTrust consent overlay on first visit
//	  - After OneTrust dismissed: shows <g id="green-button"> SVG icon
//	  - Clicking the icon reveals the date-range export form
//
// The GreenButton VF iframe is plain HTML (no shadow DOM), so querySelector
// works inside it. OneTrust must be dismissed before g#green-button or the
// form inputs appear.
func (fd *formDriver) drive(ctx context.Context, from, to time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, exportFormTimeout)
	defer cancel()

	// Pre-set OneTrust consent cookies so the overlay never appears when the VF
	// iframe loads. Network.setCookie writes at the browser level — no JS
	// required, domain-origin restrictions don't apply, and the cookies are
	// visible to the VF iframe as soon as it starts loading.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		exp := cdp.TimeSinceEpoch(time.Now().Add(365 * 24 * time.Hour))
		cookies := []*cdpnetwork.CookieParam{
			{
				Name:    "OptanonAlertBoxClosed",
				Value:   time.Now().UTC().Format(time.RFC3339),
				Domain:  "myaccount.pge.com",
				Path:    "/",
				Expires: &exp,
			},
			{
				Name:    "OptanonConsent",
				Value:   "isGpcEnabled=0&groups=C0001%3A1%2CC0002%3A1%2CC0003%3A1%2CC0004%3A1&interactionCount=1",
				Domain:  "myaccount.pge.com",
				Path:    "/",
				Expires: &exp,
			},
		}
		return cdpnetwork.SetCookies(cookies).Do(ctx)
	})); err != nil {
		fd.debug("setting OneTrust cookies: %v", err)
	}

	// Inject the OneTrust auto-dismiss script as belt-and-suspenders backup for
	// sites that re-show the banner even when cookies are present.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(oneTrustDismissScript).Do(ctx)
		return err
	})); err != nil {
		fd.debug("addScriptToEvaluateOnNewDocument: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(greenButtonURL)); err != nil {
		return fmt.Errorf("navigating to green button page: %w", err)
	}

	// If PG&E redirected to the login page (session expired or first run without
	// a cached session), scroll to the top so the username/password form is
	// visible, then wait for the user to authenticate before touching the page.
	// Scrolling on the login page makes credential entry impossible.
	time.Sleep(500 * time.Millisecond)
	if u, _ := currentURL(ctx); !urlPathContains(u, readyURLMarker) {
		report(fd.notify, "PG&E is asking for sign-in again. Please log in (including any 2FA); automation will continue once the usage page loads…")
		if err := awaitLoginScrollingToTop(ctx, readyURLMarker, loginTimeout); err != nil {
			return fmt.Errorf("waiting for re-authentication: %w", err)
		}
	}

	// Scroll to trigger Salesforce's lazy-loading intersection observer.
	// Salesforce Lightning uses a custom scroll container (main, .siteBody, etc.) —
	// window.scrollBy alone does not reach it, so we target every plausible
	// container and repeat in the poll loop below.
	_ = chromedp.Run(ctx, chromedp.Evaluate(sfScrollScript, nil))
	time.Sleep(time.Second)

	// Step 1: find the GreenButton VF iframe, scrolling on every poll iteration so
	// the intersection observer that lazy-loads it keeps getting fired.
	gbFrameID, err := waitForFrameByURLWithScroll(ctx, "GreenButton", 30*time.Second)
	if err != nil {
		fd.dumpFramesAndTargets(ctx)
		return fmt.Errorf("GreenButton frame: %w", err)
	}
	if err := waitForFrameReady(ctx, gbFrameID, 20*time.Second); err != nil {
		return fmt.Errorf("GreenButton frame not ready: %w", err)
	}

	// Step 2: wait for the injected script to call window.OneTrust.AllowAll().
	// AllowAll fires consent callbacks that Opower's widget listens for before
	// rendering any content. The script removes the overlay 500 ms AFTER AllowAll
	// fires, so waiting for the SDK banner to disappear is a reliable proxy.
	// We give it 15 s (matching the script's poll timeout) then clean up any
	// remaining elements manually.
	waitForCookieBannerGone(ctx, gbFrameID, 15*time.Second)
	fd.dismissCookieBanner(ctx, gbFrameID)

	// Extra pause: Opower's widget may need a moment to finish rendering after
	// receiving the consent callback before g#green-button is clickable.
	time.Sleep(2 * time.Second)

	// Step 3: wait for g#green-button to appear (rendered after consent).
	if err := fd.waitForGreenButton(ctx, gbFrameID, 20*time.Second); err != nil {
		fd.dumpFramesAndTargets(ctx)
		fd.dumpFormElements(ctx, gbFrameID)
		return fmt.Errorf("waiting for Green Button icon: %w", err)
	}

	// Step 4: click the Green Button icon to expand the date-range form.
	if err := fd.clickGreenButton(ctx, gbFrameID); err != nil {
		return fmt.Errorf("clicking Green Button: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Step 5: find the frame that received the expanded form.
	// After clicking the Green Button icon the form may render in the same VF
	// frame or in a nested Opower sub-iframe. We search the entire subtree rooted
	// at gbFrameID so both cases are handled without hard-coding a frame URL.
	// Date inputs start disabled until "Export usage for a range of days" is
	// selected, so we wait for their presence only (not their enabled state).
	const dateSelector = `#date-selector--select-date-from, #date-selector--select-date-to, ` +
		`input[name='fromDate'], input[id*='date' i][type='text']`
	formFrameID, err := fd.findFormFrame(ctx, gbFrameID, dateSelector, 30*time.Second)
	if err != nil {
		fd.dumpFormElements(ctx, gbFrameID)
		return fmt.Errorf("waiting for date form inputs: %w", err)
	}

	// Step 6: select "Export usage for a range of days" to enable date inputs.
	if err := jsClickInFrame(ctx, formFrameID, `#period-date, input[value='period-date']`); err != nil {
		return fmt.Errorf("selecting date range radio: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Step 7: fill from/to dates (inputs now enabled).
	if err := setTextInFrame(ctx, formFrameID,
		`#date-selector--select-date-from, input[name='fromDate']`,
		from.Format("01/02/2006")); err != nil {
		return fmt.Errorf("setting from date: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := setTextInFrame(ctx, formFrameID,
		`#date-selector--select-date-to, input[name='toDate']`,
		to.Format("01/02/2006")); err != nil {
		return fmt.Errorf("setting to date: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Step 7b: select XML format (ESPI Atom; richer than CSV — includes import and
	// export channels separately).
	if err := jsClickInFrame(ctx, formFrameID, `#xml, input[value='xml']`); err != nil {
		fd.debug("selecting XML radio: %v (continuing)", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Step 8: click Export. The button has no stable id; try class-based selectors
	// first, then fall back to matching by text content via a JS scan.
	if err := jsClickInFrame(ctx, formFrameID,
		`button.primary, button[class*='primary'], #export-btn`); err != nil {
		fallbackExpr := `(function(){
			var btns = document.querySelectorAll('button');
			for(var i=0;i<btns.length;i++){
				if((btns[i].textContent||'').trim()==='Export'){btns[i].click();return true;}
			}
			return false;
		})()`
		result, evalErr := evalInFrame(ctx, formFrameID, fallbackExpr)
		if evalErr != nil || result == nil || string(result.Value) != "true" {
			return fmt.Errorf("clicking Export button: %w", err)
		}
	}
	return nil
}

// clickGreenButton searches rootFrameID and all its descendant frames for an
// element with <g id="green-button">, then clicks its first non-SVG ancestor
// (the actual clickable element).
//
// The Green Button icon lives inside a nested cross-domain iframe that the
// Opower Visualforce wrapper page embeds — document.querySelector in the
// wrapper frame can't see into child frames, so we must search the subtree.
func (fd *formDriver) clickGreenButton(ctx context.Context, rootFrameID cdp.FrameID) error {
	targetFrameID, err := fd.findContainingFrame(ctx, rootFrameID, "g#green-button")
	if err != nil {
		fd.dumpFramesAndTargets(ctx)
		return fmt.Errorf("g#green-button not found in frame %s or descendants: %w", rootFrameID, err)
	}
	fd.debug("found g#green-button in frame %s", targetFrameID)

	// Get this frame's viewport origin (handles cross-domain nested iframes).
	originX, originY, err := fd.iframeContentOrigin(ctx, targetFrameID)
	if err != nil {
		return fmt.Errorf("Green Button frame origin: %w", err)
	}

	// g#green-button may be inside an open shadow root (Opower uses Web Components).
	// deepQuery pierces shadow roots the same way findContainingFrame does.
	// We walk up from the <g> to its nearest <button>/<a>/[role=button] ancestor so
	// the click fires on the real interactive element; clicking a raw <g> SVG node
	// does not bubble properly through Opower's event listeners.
	clickExpr := `(function(){
		try {
			function deepQuery(root, sel) {
				var el = root.querySelector(sel);
				if (el) return el;
				var nodes = root.querySelectorAll('*');
				for (var i = 0; i < nodes.length; i++) {
					if (nodes[i].shadowRoot) {
						var f = deepQuery(nodes[i].shadowRoot, sel);
						if (f) return f;
					}
				}
				return null;
			}
			var g = deepQuery(document, 'g#green-button');
			if (!g) return {err:'no g#green-button (deep)'};
			// Walk up to the nearest clickable ancestor so the synthetic click
			// reaches Opower's event handler (clicking <g> alone does not trigger it).
			var clickTarget = g;
			for (var a = g.parentElement; a; a = a.parentElement) {
				var tag = (a.tagName||'').toUpperCase();
				if (tag==='BUTTON'||tag==='A'||a.getAttribute('role')==='button') {
					clickTarget = a;
					break;
				}
			}
			clickTarget.scrollIntoView({block:'center', inline:'center'});
			var r = clickTarget.getBoundingClientRect();
			try { clickTarget.click(); } catch(e2) {}
			return {tag: clickTarget.tagName, x: r.left + r.width/2, y: r.top + r.height/2};
		} catch(e) {
			return {err: String(e)};
		}
	})()`
	result, err := evalInFrame(ctx, targetFrameID, clickExpr)
	if err != nil {
		return fmt.Errorf("clicking Green Button via JS: %w", err)
	}
	if result == nil || string(result.Value) == "null" {
		return fmt.Errorf("g#green-button not found in frame %s", targetFrameID)
	}
	var info struct {
		Err string  `json:"err"`
		Tag string  `json:"tag"`
		X   float64 `json:"x"`
		Y   float64 `json:"y"`
	}
	if err := json.Unmarshal([]byte(result.Value), &info); err != nil {
		return fmt.Errorf("parsing Green Button click result: %w", err)
	}
	if info.Err != "" {
		return fmt.Errorf("Green Button JS click: %s", info.Err)
	}
	fd.debug("clickGreenButton: .click() on <%s> at iframe-local (%.0f,%.0f) => viewport (%.0f,%.0f)",
		info.Tag, info.X, info.Y, originX+info.X, originY+info.Y)
	_ = dispatchClick(ctx, originX+info.X, originY+info.Y)
	return nil
}

// waitForGreenButton polls until g#green-button is present somewhere in
// rootFrameID or any of its descendant frames, or timeout elapses.
func (fd *formDriver) waitForGreenButton(ctx context.Context, rootFrameID cdp.FrameID, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := fd.findContainingFrame(ctx, rootFrameID, "g#green-button"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("g#green-button did not appear in frame %s or descendants within %s", rootFrameID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// findContainingFrame searches rootFrameID and all descendant frames (DFS) for
// the first frame whose document contains selector, including inside open shadow
// roots. If rootFrameID is empty, the entire frame tree is searched.
func (fd *formDriver) findContainingFrame(ctx context.Context, rootFrameID cdp.FrameID, selector string) (cdp.FrameID, error) {
	var tree *page.FrameTree
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = page.GetFrameTree().Do(ctx)
		return err
	})); err != nil {
		return "", fmt.Errorf("getting frame tree: %w", err)
	}

	// Determine the subtree to search.
	subtree := tree
	if rootFrameID != "" {
		var findSubtree func(*page.FrameTree) *page.FrameTree
		findSubtree = func(ft *page.FrameTree) *page.FrameTree {
			if ft.Frame.ID == rootFrameID {
				return ft
			}
			for _, child := range ft.ChildFrames {
				if found := findSubtree(child); found != nil {
					return found
				}
			}
			return nil
		}
		if sub := findSubtree(tree); sub != nil {
			subtree = sub
		} else {
			fd.debug("root frame %s not in tree, searching entire tree", rootFrameID)
		}
	}

	// Pierce open shadow roots via a recursive helper so elements inside Web
	// Components are found even if querySelector can't reach them directly.
	expr := fmt.Sprintf(`(function deepFind(root,sel){
		if(root.querySelector(sel)) return true;
		var all=root.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].shadowRoot && deepFind(all[i].shadowRoot,sel)) return true;
		}
		return false;
	})(document,%q)`, selector)

	var search func(*page.FrameTree) cdp.FrameID
	search = func(ft *page.FrameTree) cdp.FrameID {
		result, err := evalInFrame(ctx, ft.Frame.ID, expr)
		if err != nil {
			fd.debug("findContainingFrame: evalInFrame %s (%s): %v",
				ft.Frame.ID, ft.Frame.URL, err)
		} else if result != nil && string(result.Value) == "true" {
			fd.debug("found %q in frame %s (%s)",
				selector, ft.Frame.ID, ft.Frame.URL)
			return ft.Frame.ID
		}
		for _, child := range ft.ChildFrames {
			if id := search(child); id != "" {
				return id
			}
		}
		return ""
	}
	if id := search(subtree); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("selector %q not found in frame %s or any of its descendants", selector, rootFrameID)
}

// findFormFrame searches rootFrameID and all its descendant frames for the first
// frame that contains an element matching selector, then returns that frame's ID.
// It polls until timeout elapses. Used to locate the expanded Green Button date
// form, which may render in the VF frame itself or in a nested Opower sub-iframe.
func (fd *formDriver) findFormFrame(ctx context.Context, rootFrameID cdp.FrameID, selector string, timeout time.Duration) (cdp.FrameID, error) {
	deadline := time.Now().Add(timeout)
	for {
		frameID, err := fd.findContainingFrame(ctx, rootFrameID, selector)
		if err == nil {
			return frameID, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no element matched %q in frame %s or descendants within %s", selector, rootFrameID, timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// dismissCookieBanner removes OneTrust overlay elements directly from the DOM
// inside frameID. DOM removal doesn't require isTrusted events — it bypasses
// all of OneTrust's click-handler guards. Best-effort: logs what was removed.
func (fd *formDriver) dismissCookieBanner(ctx context.Context, frameID cdp.FrameID) {
	removeExpr := `(function() {
		var removed = [];
		['onetrust-consent-sdk','onetrust-banner-sdk','onetrust-pc-sdk',
		 'onetrust-reject-all-handler-container'].forEach(function(id){
			var el = document.getElementById(id);
			if (el && el.parentNode) { el.parentNode.removeChild(el); removed.push('#'+id); }
		});
		document.querySelectorAll(
			'.onetrust-pc-dark-filter,.onetrust-overlay,.ot-fade-in,.ot-sdk-container'
		).forEach(function(el){
			if (el.parentNode) { el.parentNode.removeChild(el); removed.push('.'+el.className.split(' ')[0]); }
		});
		if (removed.length && document.body) {
			document.body.style.overflow = '';
			document.body.style.position = '';
		}
		return removed.length > 0 ? removed.join(',') : 'nothing-to-remove';
	})()`
	result, err := evalInFrame(ctx, frameID, removeExpr)
	if err != nil {
		fd.debug("dismissCookieBanner remove: %v", err)
	} else if result != nil {
		fd.debug("dismissCookieBanner removed: %s", result.Value)
	}
}

// iframeContentOrigin returns the VIEWPORT coordinates of the top-left content
// corner of the iframe that owns frameID.
//
// DOM.getFrameOwner bypasses closed shadow DOM roots (Salesforce LWC) to reach
// the <iframe> element. DOM.getBoxModel returns page-layout coordinates, which
// must be converted to viewport coordinates by subtracting the outer page's
// current scroll offset — Input.dispatchMouseEvent uses viewport coordinates.
//
// We also scroll the iframe into view first so that the element is actually
// inside the viewport, making clicks reachable.
func (fd *formDriver) iframeContentOrigin(ctx context.Context, frameID cdp.FrameID) (x, y float64, err error) {
	var backendNodeID cdp.BackendNodeID
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		backendNodeID, _, e = cdpdom.GetFrameOwner(frameID).Do(ctx)
		return e
	})); err != nil {
		return 0, 0, fmt.Errorf("getFrameOwner: %w", err)
	}

	// Scroll the iframe into the viewport; wait for the scroll to settle.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return cdpdom.ScrollIntoViewIfNeeded().WithBackendNodeID(backendNodeID).Do(ctx)
	})); err != nil {
		fd.debug("scrollIntoViewIfNeeded: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	var model *cdpdom.BoxModel
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		model, e = cdpdom.GetBoxModel().WithBackendNodeID(backendNodeID).Do(ctx)
		return e
	})); err != nil {
		return 0, 0, fmt.Errorf("getBoxModel for iframe: %w", err)
	}
	q := model.Content // [x0,y0, x1,y1, x2,y2, x3,y3] — top-left is [0],[1]
	pageX, pageY := q[0], q[1]

	// Subtract the outer page scroll offset to convert page→viewport coordinates.
	var scrollX, scrollY float64
	_ = chromedp.Run(ctx, chromedp.Evaluate(`window.scrollX`, &scrollX))
	_ = chromedp.Run(ctx, chromedp.Evaluate(`window.scrollY`, &scrollY))
	return pageX - scrollX, pageY - scrollY, nil
}

// dumpFormElements logs all form controls in frameID to the diagnostic buffer.
// Called only in failure paths so the output is included in the flush to stderr.
func (fd *formDriver) dumpFormElements(ctx context.Context, frameID cdp.FrameID) {
	expr := `(function(){
		function collectEls(root, out) {
			root.querySelectorAll('input, select, button, textarea, a').forEach(function(el){
				out.push({tag:el.tagName, type:el.getAttribute('type')||'', id:el.id,
				          name:el.name||'', val:el.value||'',
				          text:(el.textContent||'').trim().substring(0,60),
				          ph:el.placeholder||''});
			});
			root.querySelectorAll('*').forEach(function(el){
				if(el.shadowRoot) collectEls(el.shadowRoot, out);
			});
		}
		var out=[];
		collectEls(document, out);
		return JSON.stringify(out);
	})()`
	result, err := evalInFrame(ctx, frameID, expr)
	if err != nil {
		fd.debug("dumpFormElements error: %v", err)
		return
	}
	fd.debug("elements in Green Button frame:\n%s", string(result.Value))
}

// dumpFramesAndTargets appends the page's frame tree and all browser targets to
// the diagnostic buffer. Called only in failure paths.
func (fd *formDriver) dumpFramesAndTargets(ctx context.Context) {
	fd.debug("frame tree:")
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		tree, err := page.GetFrameTree().Do(ctx)
		if err != nil {
			fd.debug("  (error: %v)", err)
			return nil
		}
		var dump func(*page.FrameTree, int)
		dump = func(ft *page.FrameTree, depth int) {
			fd.debug("%s[frame] id=%s url=%s",
				strings.Repeat("  ", depth), ft.Frame.ID, ft.Frame.URL)
			for _, child := range ft.ChildFrames {
				dump(child, depth+1)
			}
		}
		dump(tree, 1)
		return nil
	}))
	fd.debug("targets:")
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		targets, err := cdptarget.GetTargets().Do(ctx)
		if err != nil {
			fd.debug("  (error: %v)", err)
			return nil
		}
		for _, t := range targets {
			fd.debug("  type=%-10s attached=%-5v url=%s", t.Type, t.Attached, t.URL)
		}
		return nil
	}))
}

// ---- Frame tree helpers -------------------------------------------------------

// findFrameByURLInTree performs a depth-first search of tree for the first frame
// whose URL contains urlSubstr. It returns the matching frame ID, or the empty
// string if none is found. This pure function is the testable core of
// findFrameByURL.
func findFrameByURLInTree(tree *page.FrameTree, urlSubstr string) cdp.FrameID {
	if strings.Contains(tree.Frame.URL, urlSubstr) {
		return tree.Frame.ID
	}
	for _, child := range tree.ChildFrames {
		if id := findFrameByURLInTree(child, urlSubstr); id != "" {
			return id
		}
	}
	return ""
}

// findFrameByURL fetches the current frame tree from CDP and returns the first
// frame whose URL contains urlSubstr.
func findFrameByURL(ctx context.Context, urlSubstr string) (cdp.FrameID, error) {
	var tree *page.FrameTree
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = page.GetFrameTree().Do(ctx)
		return err
	})); err != nil {
		return "", fmt.Errorf("getting frame tree: %w", err)
	}
	if id := findFrameByURLInTree(tree, urlSubstr); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("no frame with URL containing %q found in frame tree", urlSubstr)
}

// ---- Poll helpers -------------------------------------------------------------

// waitForCookieBannerGone polls until OneTrust's overlay root is removed from
// the DOM inside frameID, or timeout elapses. Best-effort: returns on timeout
// so the caller can still attempt the Green Button click.
func waitForCookieBannerGone(ctx context.Context, frameID cdp.FrameID, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		result, err := evalInFrame(ctx, frameID,
			`!document.querySelector('#onetrust-consent-sdk, #onetrust-banner-sdk, .onetrust-pc-dark-filter')`)
		if err == nil && result != nil && string(result.Value) == "true" {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// waitForFrameByURLWithScroll polls the CDP frame tree until a frame whose URL
// contains urlSubstr appears, re-firing sfScrollScript on every poll tick so
// Salesforce's lazy-load intersection observer keeps getting triggered.
func waitForFrameByURLWithScroll(ctx context.Context, urlSubstr string, timeout time.Duration) (cdp.FrameID, error) {
	deadline := time.Now().Add(timeout)
	for {
		if id, err := findFrameByURL(ctx, urlSubstr); err == nil {
			return id, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no frame with URL containing %q appeared within %s", urlSubstr, timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
			_ = chromedp.Run(ctx, chromedp.Evaluate(sfScrollScript, nil))
		}
	}
}

// waitForFrameByURL polls the CDP frame tree until a frame whose URL contains
// urlSubstr appears or the timeout elapses.
func waitForFrameByURL(ctx context.Context, urlSubstr string, timeout time.Duration) (cdp.FrameID, error) {
	deadline := time.Now().Add(timeout)
	for {
		if id, err := findFrameByURL(ctx, urlSubstr); err == nil {
			return id, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no frame with URL containing %q appeared within %s", urlSubstr, timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// waitForFrameReady polls until document.readyState === 'complete' inside
// frameID, or until timeout elapses.
func waitForFrameReady(ctx context.Context, frameID cdp.FrameID, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		result, err := evalInFrame(ctx, frameID, `document.readyState`)
		if err == nil && result != nil && string(result.Value) == `"complete"` {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("document.readyState never became 'complete' in frame %s within %s", frameID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// ---- JS evaluation and interaction -------------------------------------------

// evalInFrame creates an isolated JS world in frameID and evaluates expr,
// returning the result with returnByValue=true. CreateIsolatedWorld gives a
// context ID synchronously — no event listening needed.
func evalInFrame(ctx context.Context, frameID cdp.FrameID, expr string) (*cdpruntime.RemoteObject, error) {
	var contextID cdpruntime.ExecutionContextID
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		contextID, e = page.CreateIsolatedWorld(frameID).Do(ctx)
		return e
	})); err != nil {
		return nil, fmt.Errorf("creating isolated world for frame %s: %w", frameID, err)
	}
	var result *cdpruntime.RemoteObject
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var exc *cdpruntime.ExceptionDetails
		var e error
		result, exc, e = cdpruntime.Evaluate(expr).
			WithContextID(contextID).
			WithReturnByValue(true).
			WithUserGesture(true).
			Do(ctx)
		if e != nil {
			return e
		}
		if exc != nil {
			desc := exc.Text
			if exc.Exception != nil && exc.Exception.Description != "" {
				desc = exc.Exception.Description
			}
			return fmt.Errorf("JS exception in frame: %s", desc)
		}
		return nil
	})); err != nil {
		return nil, err
	}
	return result, nil
}

// jsClickInFrame JS-clicks the first element matching selector inside frameID.
// Unlike physical mouse dispatch, this works even when the iframe is scrolled
// outside the main-page viewport (negative Y origin — common on Salesforce
// Lightning pages that use a custom scroll container).
func jsClickInFrame(ctx context.Context, frameID cdp.FrameID, selector string) error {
	expr := fmt.Sprintf(`(function() {
		%s
		var candidates = %q.split(", ");
		for (var i = 0; i < candidates.length; i++) {
			var el = deepQuery(document, candidates[i].trim());
			if (el) {
				el.scrollIntoView({block:'center'});
				el.click();
				return true;
			}
		}
		return false;
	})()`, deepQueryJS, selector)
	result, err := evalInFrame(ctx, frameID, expr)
	if err != nil {
		return err
	}
	if result == nil || string(result.Value) != "true" {
		return fmt.Errorf("no element matched %q in frame %s", selector, frameID)
	}
	return nil
}

// setTextInFrame sets the value of the first element matching selector inside
// frameID using JS, then fires input/change/blur events so the framework
// (Opower uses plain DOM listeners, not React) picks up the new value.
//
// CDP keyboard simulation (InsertText) was tried first but is unreliable here:
// when the VF iframe is scrolled above the main-page viewport (negative Y
// origin — common on Salesforce Lightning with custom scroll containers),
// isolated-world focus calls do not give the element OS-level browser focus,
// so CDP keyboard events land in the wrong frame. Pure JS value-setting works
// because Opower's form does not gate events on isTrusted.
func setTextInFrame(ctx context.Context, frameID cdp.FrameID, selector, value string) error {
	expr := fmt.Sprintf(`(function() {
		%s
		var candidates = %q.split(", ");
		for (var i = 0; i < candidates.length; i++) {
			var el = deepQuery(document, candidates[i].trim());
			if (el) {
				// Use the native HTMLInputElement setter so React-controlled
				// inputs also see the change (not needed for Opower, but harmless).
				try {
					var setter = Object.getOwnPropertyDescriptor(
						Object.getPrototypeOf(el), 'value').set;
					setter.call(el, %q);
				} catch(e) { el.value = %q; }
				el.dispatchEvent(new Event('input',  {bubbles:true}));
				el.dispatchEvent(new Event('change', {bubbles:true}));
				el.dispatchEvent(new Event('blur',   {bubbles:true}));
				return true;
			}
		}
		return false;
	})()`, deepQueryJS, selector, value, value)
	result, err := evalInFrame(ctx, frameID, expr)
	if err != nil {
		return err
	}
	if result == nil || string(result.Value) != "true" {
		return fmt.Errorf("no element matched %q in frame %s", selector, frameID)
	}
	return nil
}

// dispatchClick fires a left mousePressed + mouseReleased pair at (x, y) in
// page viewport coordinates.
func dispatchClick(ctx context.Context, x, y float64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := input.DispatchMouseEvent(input.MousePressed, x, y).
			WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
			return err
		}
		return input.DispatchMouseEvent(input.MouseReleased, x, y).
			WithButton(input.Left).WithClickCount(1).Do(ctx)
	}))
}
