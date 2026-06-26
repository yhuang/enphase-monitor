package enphase

import "testing"

func TestParseAppList(t *testing.T) {
	// Mirrors the real list-page anchor markup:
	// <a href="/admin/applications/<id>">enphase-monitor-0NN</a>, plus deeper
	// links (.../edit) that must be ignored, and a duplicate link.
	const listHTML = `
<table>
  <tr><td><a href="/admin/applications/1409625149340">enphase-monitor-40</a>
      <a href="/admin/applications/1409625149340/edit">edit</a></td></tr>
  <tr><td><a href="/admin/applications/1409626578864">enphase-monitor-01</a></td></tr>
  <tr><td><a href="/admin/applications/1409625149340">dup</a></td></tr>
</table>`

	apps := parseAppList(listHTML)
	if len(apps) != 2 {
		t.Fatalf("parseAppList() returned %d apps, want 2: %+v", len(apps), apps)
	}
	want := []portalApp{
		{name: "enphase-monitor-40", path: "/admin/applications/1409625149340"},
		{name: "enphase-monitor-01", path: "/admin/applications/1409626578864"},
	}
	for i, w := range want {
		if apps[i] != w {
			t.Errorf("apps[%d] = %+v, want %+v", i, apps[i], w)
		}
	}
}

func TestDetailPageExtraction(t *testing.T) {
	// Mirrors the inline script values on an application's detail page.
	const detailHTML = `
  <dt> API Key</dt><dd><code id="user-key">f9ece7a2d940ea0b6b1fa3ea11b7dc64</code></dd>
  <script>
  var key = "f9ece7a2d940ea0b6b1fa3ea11b7dc64";
  var email = "jimmy.huang@duragility.com";
  </script>`

	if got := firstSubmatch(keyPattern, detailHTML); got != "f9ece7a2d940ea0b6b1fa3ea11b7dc64" {
		t.Errorf("key = %q, want f9ece7a2d940ea0b6b1fa3ea11b7dc64", got)
	}
	if got := firstSubmatch(emailPattern, detailHTML); got != "jimmy.huang@duragility.com" {
		t.Errorf("email = %q, want jimmy.huang@duragility.com", got)
	}
	if got := firstSubmatch(keyPattern, "<p>no script here</p>"); got != "" {
		t.Errorf("key from empty page = %q, want empty", got)
	}
}
