package pge

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
)

// frame is a helper that builds a *page.FrameTree node for use in tests.
func frame(id, url string, children ...*page.FrameTree) *page.FrameTree {
	return &page.FrameTree{
		Frame:       &cdp.Frame{ID: cdp.FrameID(id), URL: url},
		ChildFrames: children,
	}
}

func TestFindFrameByURLInTree(t *testing.T) {
	tests := []struct {
		name      string
		tree      *page.FrameTree
		urlSubstr string
		wantID    cdp.FrameID
	}{
		{
			name:      "root matches",
			tree:      frame("root", "https://apex.pge.com/apex/myAcct_VF_GreenButton"),
			urlSubstr: "GreenButton",
			wantID:    "root",
		},
		{
			name:      "no match returns empty",
			tree:      frame("root", "https://example.com/"),
			urlSubstr: "GreenButton",
			wantID:    "",
		},
		{
			name: "direct child matches",
			tree: frame("root", "https://myaccount.pge.com/",
				frame("child", "https://apex.pge.com/apex/myAcct_VF_GreenButton"),
			),
			urlSubstr: "GreenButton",
			wantID:    "child",
		},
		{
			name: "nested grandchild matches",
			tree: frame("root", "https://myaccount.pge.com/",
				frame("mid", "https://myaccount.pge.com/page",
					frame("grand", "https://apex.pge.com/apex/myAcct_VF_GreenButton"),
				),
			),
			urlSubstr: "GreenButton",
			wantID:    "grand",
		},
		{
			name: "first DFS match wins",
			tree: frame("root", "https://myaccount.pge.com/",
				frame("c1", "https://apex.pge.com/apex/myAcct_VF_GreenButton"),
				frame("c2", "https://other.pge.com/apex/myAcct_VF_GreenButton"),
			),
			urlSubstr: "GreenButton",
			wantID:    "c1",
		},
		{
			name: "partial URL match is sufficient",
			tree: frame("root", "https://myaccount.pge.com/",
				frame("child", "https://apex.pge.com/apex/myAcct_VF_GreenButton?param=1"),
			),
			urlSubstr: "GreenButton",
			wantID:    "child",
		},
		{
			name: "sibling after no-match is checked",
			tree: frame("root", "https://myaccount.pge.com/",
				frame("c1", "https://example.com/unrelated"),
				frame("c2", "https://apex.pge.com/apex/myAcct_VF_GreenButton"),
			),
			urlSubstr: "GreenButton",
			wantID:    "c2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findFrameByURLInTree(tt.tree, tt.urlSubstr)
			if got != tt.wantID {
				t.Errorf("findFrameByURLInTree: got %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestFormDriverDebugBuffer(t *testing.T) {
	fd := &formDriver{}

	// Buffer starts empty.
	if fd.buf.Len() != 0 {
		t.Fatal("new formDriver should have an empty buffer")
	}

	fd.debug("hello %s", "world")
	fd.debug("second line")

	if fd.buf.Len() == 0 {
		t.Fatal("debug output not buffered")
	}
	raw := fd.buf.String()
	if !strings.Contains(raw, "hello world") {
		t.Errorf("buffer missing 'hello world', got: %q", raw)
	}
	if !strings.Contains(raw, "second line") {
		t.Errorf("buffer missing 'second line', got: %q", raw)
	}
	if !strings.Contains(raw, "[pge/form]") {
		t.Errorf("buffer missing '[pge/form]' prefix, got: %q", raw)
	}
}

func TestFormDriverFlushTo(t *testing.T) {
	fd := &formDriver{}
	fd.debug("line one")
	fd.debug("line two")

	var out strings.Builder
	fd.flushTo(&out)

	if !strings.Contains(out.String(), "line one") {
		t.Errorf("flushed output missing 'line one': %q", out.String())
	}
	if !strings.Contains(out.String(), "line two") {
		t.Errorf("flushed output missing 'line two': %q", out.String())
	}

	// Buffer is drained after flush; a second flush writes nothing.
	var out2 strings.Builder
	fd.flushTo(&out2)
	if out2.Len() != 0 {
		t.Errorf("second flush should write nothing, got: %q", out2.String())
	}
}

func TestFormDriverFlushEmpty(t *testing.T) {
	fd := &formDriver{}
	var out strings.Builder
	fd.flushTo(&out) // must not panic on empty buffer
	if out.Len() != 0 {
		t.Errorf("flush of empty buffer wrote bytes: %q", out.String())
	}
}
