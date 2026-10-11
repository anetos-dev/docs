// Command versions plans the sites build.sh builds, one per version of
// the docs, from the framework's tags:
//
//	go run ./versions -repo https://github.com/anetos-dev/anetos -out .versions
//
// Each minor version from v0.5 with a release tag (not a pre-release)
// gets a site, built from its newest patch tag. The newest is the root
// (docs.anetos.dev/), main is at /main/, and older ones at /v0.5/…. While
// there is no release, main is the root and nothing else is built.
//
// It writes into -out:
//
//   - sites.tsv: one line per site, "name ref dir" (dir "." for the
//     root), the root first;
//   - <name>.json: the site's Hugo configuration, merged over hugo.yaml
//     (hugo --config hugo.yaml,<name>.json): its baseURL, its version
//     (params.version, which the layouts read), the ref its links to
//     code point at, its edit links, its banner, and the navbar menu
//     with the versions (Hugo replaces a menu rather than merging it,
//     so this one has hugo.yaml's items too, from `hugo config`);
//   - _redirects: lines for Cloudflare Pages' _redirects, sending the
//     latest version's own address (/v0.6/…) to the root;
//   - _headers: static/_headers' rules for paths under the root (cache
//     lifetimes), repeated for the sites under /main/ and /v0.N/, since
//     Cloudflare Pages reads _headers at the root only.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// first is the first minor version with docs: v0.5.0 is the first
// public release.
var first = semver{Major: 0, Minor: 5}

func main() {
	repo := flag.String("repo", "https://github.com/anetos-dev/anetos", "the framework's git repository (a URL or a path)")
	out := flag.String("out", ".versions", "the folder to write the plan into (replaced)")
	base := flag.String("base", "https://docs.anetos.dev/", "the site's address")
	hugo := flag.String("hugo", "hugo", "the hugo command, which reads hugo.yaml's menu")
	headers := flag.String("headers", "static/_headers", "the root's _headers, whose rules the other sites get too")
	flag.Parse()
	if err := run(*repo, *out, *base, *hugo, *headers); err != nil {
		fmt.Fprintln(os.Stderr, "versions:", err)
		os.Exit(1)
	}
}

func run(repo, out, base, hugo, headers string) error {
	cmd := exec.Command("git", "ls-remote", "--tags", repo)
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git ls-remote %s: %w", repo, err)
	}
	sites := plan(tags(b), base)
	var menu []any
	if len(sites) > 1 {
		if menu, err = hugoMenu(hugo); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	var tsv, redirects bytes.Buffer
	for _, s := range sites {
		fmt.Fprintf(&tsv, "%s %s %s\n", s.Name, s.Ref, s.dir())
		b, err := json.MarshalIndent(config(s, sites, menu), "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, s.Name+".json"), append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "versions: %-5s from %-12s at %s\n", s.Name, s.Ref, s.URL)
	}
	for _, from := range alias(sites) {
		fmt.Fprintf(&redirects, "/%s / 302\n/%s/* /:splat 302\n", from, from)
	}
	if err := os.WriteFile(filepath.Join(out, "sites.tsv"), tsv.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "_redirects"), redirects.Bytes(), 0o644); err != nil {
		return err
	}
	rules, err := os.ReadFile(headers)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "_headers"), nestedHeaders(rules, sites), 0o644)
}

// nestedHeaders repeats the rules of a _headers file for the sites below
// the root: "/css/*" becomes "/main/css/*" and "/v0.5/css/*". A rule for
// every path ("/*") already covers them, and comments are left out.
func nestedHeaders(rules []byte, sites []site) []byte {
	type rule struct {
		path    string
		headers []string
	}
	var rs []rule
	for _, line := range strings.Split(string(rules), "\n") {
		switch {
		case strings.HasPrefix(line, "/"):
			rs = append(rs, rule{path: strings.TrimSpace(line)})
		case strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#"):
		case len(rs) > 0:
			rs[len(rs)-1].headers = append(rs[len(rs)-1].headers, line)
		}
	}
	var b bytes.Buffer
	for _, s := range sites {
		if s.dir() == "." {
			continue
		}
		for _, r := range rs {
			if r.path == "/*" || len(r.headers) == 0 {
				continue
			}
			fmt.Fprintf(&b, "/%s%s\n%s\n", s.dir(), r.path, strings.Join(r.headers, "\n"))
		}
	}
	return b.Bytes()
}

// tags reads git ls-remote's output: the tags of the core module
// (vX.Y.Z, not cli/vX.Y.Z), parsed.
func tags(lsRemote []byte) []semver {
	var vs []semver
	sc := bufio.NewScanner(bytes.NewReader(lsRemote))
	for sc.Scan() {
		_, ref, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok || strings.HasSuffix(name, "^{}") { // an annotated tag's commit
			continue
		}
		if v, ok := parse(name); ok {
			vs = append(vs, v)
		}
	}
	return vs
}

// A site is one version of the docs.
type site struct {
	Name string // "v0.6", "main"
	Ref  string // the git ref its pages come from: "v0.6.1", "main"
	URL  string // its address: "https://docs.anetos.dev/main/"
	// Kind is "latest" (the root, the newest release), "main" (main
	// beside a release), "older" (an older release) or "unreleased" (main
	// at the root, before the first release).
	Kind       string
	Latest     string // the newest release's name ("v0.6"); "" before the first
	Prerelease string // the newest pre-release tag newer than every release: "v0.7.0-rc.1"
}

func (s site) dir() string {
	switch s.Kind {
	case "latest", "unreleased":
		return "."
	}
	return s.Name
}

// plan returns the sites to build, the root first, then main, then the
// releases from the newest.
func plan(vs []semver, base string) []site {
	newest := map[[2]int]semver{} // the newest patch release of each minor version
	var pre semver                // the newest pre-release
	for _, v := range vs {
		if cmpInts([]int{v.Major, v.Minor}, []int{first.Major, first.Minor}) < 0 {
			continue
		}
		if v.Pre != "" {
			if pre.Pre == "" || pre.less(v) {
				pre = v
			}
			continue
		}
		k := [2]int{v.Major, v.Minor}
		if n, ok := newest[k]; !ok || n.less(v) {
			newest[k] = v
		}
	}
	var releases []semver
	for _, v := range newest {
		releases = append(releases, v)
	}
	slices.SortFunc(releases, func(a, b semver) int {
		switch {
		case b.less(a):
			return -1
		case a.less(b):
			return 1
		}
		return 0
	})
	prerelease := ""
	if pre.Pre != "" && (len(releases) == 0 || releases[0].less(pre)) {
		prerelease = pre.String()
	}
	if len(releases) == 0 {
		return []site{{Name: "main", Ref: "main", URL: base, Kind: "unreleased", Prerelease: prerelease}}
	}
	latest := releases[0].minor()
	sites := []site{
		{Name: latest, Ref: releases[0].String(), URL: base, Kind: "latest", Latest: latest, Prerelease: prerelease},
		{Name: "main", Ref: "main", URL: base + "main/", Kind: "main", Latest: latest, Prerelease: prerelease},
	}
	for _, v := range releases[1:] {
		sites = append(sites, site{Name: v.minor(), Ref: v.String(), URL: base + v.minor() + "/", Kind: "older", Latest: latest, Prerelease: prerelease})
	}
	return sites
}

// alias returns the addresses (without slashes) that redirect to the
// root: the latest version's own (/v0.6/), and main's while main is the
// root.
func alias(sites []site) []string {
	switch sites[0].Kind {
	case "unreleased":
		return []string{"main"}
	case "latest":
		return []string{sites[0].Name}
	}
	return nil
}

// hugoMenu returns the navbar's menu (menus.main) as Hugo reads it from
// hugo.yaml.
func hugoMenu(hugo string) ([]any, error) {
	cmd := exec.Command(hugo, "config", "--format", "json")
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("hugo config: %w", err)
	}
	var cfg struct {
		Menus struct {
			Main []any `json:"main"`
		} `json:"menus"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("hugo config: %w", err)
	}
	if len(cfg.Menus.Main) == 0 {
		return nil, fmt.Errorf("hugo config: no main menu")
	}
	return cfg.Menus.Main, nil
}

// config is a site's Hugo configuration, merged over hugo.yaml; menu is
// hugo.yaml's navbar menu, which gains the version menu.
func config(s site, sites []site, menu []any) map[string]any {
	label := s.Name
	switch s.Kind {
	case "latest":
		label = s.Name + " (latest)"
	case "main", "unreleased":
		label = "main (unreleased)"
	}
	var versions []map[string]any
	for _, o := range sites {
		versions = append(versions, map[string]any{"name": o.Name, "url": o.URL, "kind": o.Kind})
	}
	latestURL := ""
	if s.Latest != "" {
		latestURL = sites[0].URL
	}
	cfg := map[string]any{
		"baseURL": s.URL,
		"params": map[string]any{
			"anetosRef": s.Ref,
			// Older versions' pages may be gone from main, where edit
			// links point.
			"editURL": map[string]any{"enable": s.Kind != "older"},
			"version": map[string]any{
				"name":       s.Name,
				"label":      label,
				"kind":       s.Kind,
				"latest":     s.Latest,
				"latestURL":  latestURL,
				"prerelease": s.Prerelease,
				"versions":   versions,
				// Search engines index the root only.
				"noindex": s.Kind == "main" || s.Kind == "older",
			},
		},
	}
	if s.Kind != "latest" {
		// The banner (layouts/_partials/custom/banner.html) says which
		// version this is. Closing it lasts until the versions change.
		key := "anetos-docs-" + s.Name + "-" + s.Latest
		if s.Kind != "older" { // the banners of main and the unreleased root name it
			key += "-" + s.Prerelease
		}
		cfg["params"].(map[string]any)["banner"] = map[string]any{"key": key}
	}
	if len(sites) > 1 {
		// Weight 100: after hugo.yaml's items, at the navbar's end.
		menu := append(slices.Clip(menu), map[string]any{"identifier": "version", "name": s.Name, "weight": 100})
		for i, o := range sites {
			name := o.Name
			switch o.Kind {
			case "latest":
				name += " (latest)"
			case "main":
				name = "main (unreleased)"
			}
			menu = append(menu, map[string]any{"name": name, "url": o.URL, "parent": "version", "weight": i + 1})
		}
		cfg["menu"] = map[string]any{"main": menu}
	}
	return cfg
}

// semver is a version vMAJOR.MINOR.PATCH[-PRE]; build metadata isn't
// accepted (the framework's tags have none).
type semver struct {
	Major, Minor, Patch int
	Pre                 string
}

func parse(s string) (semver, bool) {
	rest, ok := strings.CutPrefix(s, "v")
	if !ok {
		return semver{}, false
	}
	rest, pre, hasPre := strings.Cut(rest, "-")
	if hasPre && !validPre(pre) || strings.Contains(s, "+") {
		return semver{}, false
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var n [3]int
	for i, p := range parts {
		if p == "" || len(p) > 1 && p[0] == '0' {
			return semver{}, false
		}
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return semver{}, false
		}
		n[i] = v
	}
	return semver{n[0], n[1], n[2], pre}, true
}

// validPre reports whether s is a pre-release as semantic versioning
// defines it: dot-separated identifiers of ASCII letters, digits and
// hyphens, none empty, numeric ones without leading zeros. (The banner
// puts it in a script.)
func validPre(s string) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		numeric := true
		for _, c := range id {
			switch {
			case c >= '0' && c <= '9':
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-':
				numeric = false
			default:
				return false
			}
		}
		if numeric && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}

func (v semver) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

func (v semver) minor() string { return fmt.Sprintf("v%d.%d", v.Major, v.Minor) }

// less reports whether v precedes w, by semantic versioning's rules: a
// pre-release precedes its release, and pre-release identifiers compare
// numerically when both are numbers.
func (v semver) less(w semver) bool {
	if c := cmpInts([]int{v.Major, v.Minor, v.Patch}, []int{w.Major, w.Minor, w.Patch}); c != 0 {
		return c < 0
	}
	switch {
	case v.Pre == w.Pre:
		return false
	case v.Pre == "":
		return false
	case w.Pre == "":
		return true
	}
	a, b := strings.Split(v.Pre, "."), strings.Split(w.Pre, ".")
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		x, errX := strconv.Atoi(a[i])
		y, errY := strconv.Atoi(b[i])
		switch {
		case errX == nil && errY == nil:
			return x < y
		case errX == nil:
			return true // numeric identifiers come first
		case errY == nil:
			return false
		}
		return a[i] < b[i]
	}
	return len(a) < len(b)
}

func cmpInts(a, b []int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
