package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func lsRemote(names ...string) []byte {
	var b strings.Builder
	for i, n := range names {
		fmt.Fprintf(&b, "%040x\trefs/tags/%s\n", i, n)
	}
	return []byte(b.String())
}

func TestTags(t *testing.T) {
	in := lsRemote("v0.4.0", "cli/v0.5.0", "v0.5.0-rc.1", "v0.5.0", "v0.5.0^{}", "drivers/sqlite/v0.5.0", "release-0.5", "v1", "v0.05.0")
	var got []string
	for _, v := range tags(in) {
		got = append(got, v.String())
	}
	want := []string{"v0.4.0", "v0.5.0-rc.1", "v0.5.0"}
	if !slices.Equal(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

func TestParse(t *testing.T) {
	for _, s := range []string{"v0.5.0", "v1.2.3", "v0.5.0-rc.1", "v10.20.30-alpha.beta.1", "v0.5.0-0", "v0.5.0-x-y.0a"} {
		v, ok := parse(s)
		if !ok || v.String() != s {
			t.Errorf("parse(%q) = %v, %v", s, v, ok)
		}
	}
	for _, s := range []string{"0.5.0", "v0.5", "v0.5.0-", "v0.5.0+meta", "v0.5.0-rc.1+meta", "v01.5.0", "v0.5.x", "v-1.0.0", "",
		"v0.5.0-rc..1", "v0.5.0-rc.01", "v0.5.0-rc.", "v0.5.0-a/b", "v0.7.0-x');alert(1);('", "v0.5.0-rc_1", "v0.5.0-é"} {
		if v, ok := parse(s); ok {
			t.Errorf("parse(%q) = %v, want no version", s, v)
		}
	}
}

func TestLess(t *testing.T) {
	// In order, as semantic versioning's §11 lists them.
	order := []string{"v0.5.0-alpha", "v0.5.0-alpha.1", "v0.5.0-alpha.beta", "v0.5.0-beta", "v0.5.0-beta.2", "v0.5.0-beta.11", "v0.5.0-rc.1", "v0.5.0", "v0.5.1", "v0.6.0-rc.1", "v0.6.0", "v0.10.0", "v1.0.0"}
	for i := range order {
		for j := range order {
			a, _ := parse(order[i])
			b, _ := parse(order[j])
			if got, want := a.less(b), i < j; got != want {
				t.Errorf("%s < %s = %v, want %v", order[i], order[j], got, want)
			}
		}
	}
}

func names(sites []site) []string {
	var s []string
	for _, x := range sites {
		s = append(s, fmt.Sprintf("%s:%s:%s:%s", x.Name, x.Ref, x.dir(), x.Kind))
	}
	return s
}

func TestPlan(t *testing.T) {
	const base = "https://docs.anetos.dev/"
	parsed := func(ns ...string) []semver { return tags(lsRemote(ns...)) }
	for _, c := range []struct {
		tags       []string
		want       []string
		prerelease string
	}{
		{nil, []string{"main:main:.:unreleased"}, ""},
		// Before v0.5.0: main at the root; the old tags have no docs.
		{[]string{"v0.3.0", "v0.4.0", "v0.5.0-rc.1"}, []string{"main:main:.:unreleased"}, "v0.5.0-rc.1"},
		{[]string{"v0.4.0", "v0.5.0-rc.1", "v0.5.0-rc.2"}, []string{"main:main:.:unreleased"}, "v0.5.0-rc.2"},
		// The release.
		{[]string{"v0.4.0", "v0.5.0-rc.1", "v0.5.0"}, []string{"v0.5:v0.5.0:.:latest", "main:main:main:main"}, ""},
		// Its newest patch; a pre-release of the next.
		{[]string{"v0.5.0", "v0.5.2", "v0.5.1", "v0.6.0-rc.1"}, []string{"v0.5:v0.5.2:.:latest", "main:main:main:main"}, "v0.6.0-rc.1"},
		// A release candidate of the latest's next patch is news too;
		// pre-releases before v0.5 aren't.
		{[]string{"v0.4.1-rc.1", "v0.5.0", "v0.5.1-rc.1"}, []string{"v0.5:v0.5.0:.:latest", "main:main:main:main"}, "v0.5.1-rc.1"},
		{[]string{"v0.4.0", "v0.4.1-rc.1"}, []string{"main:main:.:unreleased"}, ""},
		// The next release: v0.5 moves to /v0.5/; an old pre-release is no news.
		{[]string{"v0.5.0", "v0.5.1", "v0.6.0-rc.1", "v0.6.0", "v0.10.0", "v0.7.0"}, []string{"v0.10:v0.10.0:.:latest", "main:main:main:main", "v0.7:v0.7.0:v0.7:older", "v0.6:v0.6.0:v0.6:older", "v0.5:v0.5.1:v0.5:older"}, ""},
	} {
		sites := plan(parsed(c.tags...), base)
		if got := names(sites); !slices.Equal(got, c.want) {
			t.Errorf("plan(%v) = %v, want %v", c.tags, got, c.want)
		}
		for _, s := range sites {
			if s.Prerelease != c.prerelease {
				t.Errorf("plan(%v): %s's prerelease = %q, want %q", c.tags, s.Name, s.Prerelease, c.prerelease)
			}
		}
	}
}

func TestConfig(t *testing.T) {
	sites := plan(tags(lsRemote("v0.5.0", "v0.5.1", "v0.6.0")), "https://docs.anetos.dev/")
	param := func(s site) map[string]any {
		return config(s, sites, nil)["params"].(map[string]any)
	}
	version := func(s site) map[string]any { return param(s)["version"].(map[string]any) }
	latest, main, older := sites[0], sites[1], sites[2]
	if got := config(main, sites, nil)["baseURL"]; got != "https://docs.anetos.dev/main/" {
		t.Errorf("main's baseURL = %v", got)
	}
	if got := param(older)["anetosRef"]; got != "v0.5.1" {
		t.Errorf("v0.5's anetosRef = %v", got)
	}
	for _, c := range []struct {
		s       site
		label   string
		noindex bool
		edit    bool
	}{
		{latest, "v0.6 (latest)", false, true},
		{main, "main (unreleased)", true, true},
		{older, "v0.5", true, false},
	} {
		v := version(c.s)
		if v["label"] != c.label || v["noindex"] != c.noindex || param(c.s)["editURL"].(map[string]any)["enable"] != c.edit {
			t.Errorf("%s: label %v, noindex %v, edit %v", c.s.Name, v["label"], v["noindex"], param(c.s)["editURL"])
		}
		if v["latestURL"] != "https://docs.anetos.dev/" || v["latest"] != "v0.6" {
			t.Errorf("%s: latest %v at %v", c.s.Name, v["latest"], v["latestURL"])
		}
	}
	base := []any{map[string]any{"name": "Guides", "pageref": "/guides/", "weight": 2}}
	menu := config(main, sites, base)["menu"].(map[string]any)["main"].([]any)
	var got []string
	for _, m := range menu {
		m := m.(map[string]any)
		got = append(got, fmt.Sprint(m["name"], " ", m["url"]))
	}
	want := []string{"Guides <nil>", "main <nil>", "v0.6 (latest) https://docs.anetos.dev/", "main (unreleased) https://docs.anetos.dev/main/", "v0.5 https://docs.anetos.dev/v0.5/"}
	if !slices.Equal(got, want) {
		t.Errorf("main's menu = %q, want %q", got, want)
	}
	if _, ok := param(latest)["banner"]; ok {
		t.Error("the latest release has a banner")
	}
	for _, s := range []site{main, older} {
		if b, ok := param(s)["banner"].(map[string]any); !ok || !strings.HasPrefix(b["key"].(string), "anetos-docs-"+s.Name+"-v0.6") {
			t.Errorf("%s's banner: %v", s.Name, param(s)["banner"])
		}
	}
	// One site, no menu.
	one := plan(nil, "https://docs.anetos.dev/")
	if _, ok := config(one[0], one, nil)["menu"]; ok {
		t.Error("a single site has a version menu")
	}
	// Before the first release, the root is indexed, and its banner names
	// the release candidate.
	rc := plan(tags(lsRemote("v0.5.0-rc.1")), "https://docs.anetos.dev/")
	p := config(rc[0], rc, nil)["params"].(map[string]any)
	v := p["version"].(map[string]any)
	if v["noindex"] != false || v["latestURL"] != "" || v["prerelease"] != "v0.5.0-rc.1" || p["banner"].(map[string]any)["key"] != "anetos-docs-main--v0.5.0-rc.1" {
		t.Errorf("the unreleased root: %v, banner %v", v, p["banner"])
	}
	// An older version's banner doesn't come back for a new pre-release.
	pre := plan(tags(lsRemote("v0.5.0", "v0.6.0", "v0.7.0-rc.1")), "https://docs.anetos.dev/")
	if k := config(pre[2], pre, nil)["params"].(map[string]any)["banner"].(map[string]any)["key"]; k != "anetos-docs-v0.5-v0.6" {
		t.Errorf("v0.5's banner key = %v", k)
	}
}

func TestAlias(t *testing.T) {
	if got := alias(plan(nil, "/")); !slices.Equal(got, []string{"main"}) {
		t.Errorf("before a release: %v", got)
	}
	if got := alias(plan(tags(lsRemote("v0.5.0", "v0.6.0")), "/")); !slices.Equal(got, []string{"v0.6"}) {
		t.Errorf("with releases: %v", got)
	}
}

func TestNestedHeaders(t *testing.T) {
	rules := "/*\n  X-Frame-Options: DENY\n\n# fingerprinted\n/css/*\n  Cache-Control: immutable\n/empty\n/en.search*\n  Cache-Control: immutable\n"
	sites := plan(tags(lsRemote("v0.5.0", "v0.6.0")), "/")
	got := string(nestedHeaders([]byte(rules), sites))
	want := "/main/css/*\n  Cache-Control: immutable\n/main/en.search*\n  Cache-Control: immutable\n/v0.5/css/*\n  Cache-Control: immutable\n/v0.5/en.search*\n  Cache-Control: immutable\n"
	if got != want {
		t.Errorf("nestedHeaders = %q, want %q", got, want)
	}
	if got := nestedHeaders([]byte(rules), plan(nil, "/")); len(got) != 0 {
		t.Errorf("one site: %q", got)
	}
}

func TestHugoMenu(t *testing.T) {
	if _, err := hugoMenu("false"); err == nil {
		t.Error("a failing hugo: no error")
	}
	if _, err := hugoMenu("true"); err == nil {
		t.Error("no output: no error")
	}
}

// TestRun runs the command on a git repository with tags.
func TestRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "one")
	for _, tag := range []string{"v0.5.0", "cli/v0.5.0", "v0.5.1", "v0.6.0-rc.1"} {
		git("tag", tag)
	}
	git("tag", "-a", "-m", "annotated", "v0.6.0")
	out := filepath.Join(t.TempDir(), "plan")
	hugo, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("no hugo")
	}
	t.Chdir("..") // hugo.yaml's folder
	if err := run(repo, out, "https://docs.anetos.dev/", hugo, "static/_headers"); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got, want := read("sites.tsv"), "v0.6 v0.6.0 .\nmain main main\nv0.5 v0.5.1 v0.5\n"; got != want {
		t.Errorf("sites.tsv = %q, want %q", got, want)
	}
	if got, want := read("_redirects"), "/v0.6 / 302\n/v0.6/* /:splat 302\n"; got != want {
		t.Errorf("_redirects = %q, want %q", got, want)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(read("v0.5.json")), &cfg); err != nil || cfg["baseURL"] != "https://docs.anetos.dev/v0.5/" {
		t.Errorf("v0.5.json: %v, %v", cfg["baseURL"], err)
	}
	if menu := fmt.Sprint(cfg["menu"]); !strings.Contains(menu, "Getting started") || !strings.Contains(menu, "identifier:version") {
		t.Errorf("v0.5.json's menu lacks hugo.yaml's items or the versions: %s", menu)
	}
	if h := read("_headers"); !strings.Contains(h, "/main/css/*\n") || !strings.Contains(h, "/v0.5/lib/*\n") {
		t.Errorf("_headers = %q", h)
	}
}
