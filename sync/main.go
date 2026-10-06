// Command sync turns the framework's docs/site folder into the site's
// content folder for Hugo:
//
//	go run ./sync -src ../anetos/docs/site -out content -files files
//
// The pages stay plain Markdown that reads well on GitHub; this adds what
// the site needs:
//
//   - README.md files become their folder's _index.md (the section page);
//     sections without one get a generated index listing their pages.
//   - Each page's leading "# Title" heading is dropped (the theme shows
//     the title), and pages without front matter get a title from it.
//   - Front matter gains, unless the page sets them: weight (the order of
//     sections in the sidebar), description (the first paragraph),
//     sourcePath (the page's path in the repository, which the link and
//     image render hooks resolve relative paths against) and editURL
//     (always on main: GitHub can't edit a file on a tag).
//   - Pages with a group in their front matter (group: "Basics") go into
//     a folder of their section named after it (content/guides/basics/),
//     which the sidebar shows as a group of its own, ordered by the
//     smallest weight of its pages; the page keeps its URL (/guides/forms/,
//     front matter url), and data/moved.json maps its old content path
//     to the new one for the link render hook. The group's folder gets
//     an index listing its pages, and a section without a README lists
//     its pages by group. The framework's docnav checks the groups.
//   - Upgrade guides (upgrade/v0.3.md) are ordered newest first, by the
//     version in their file name.
//   - Other files (images…) are copied to the -files folder, which Hugo
//     serves at their path in docs/site.
//   - The files in overlay/ (the docs' home page) are copied over the top.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// sections are the top-level folders, in sidebar order, with the titles
// of those that have no README.
var sections = []struct{ dir, title, intro string }{
	{"getting-started", "Getting started", ""},
	{"guides", "Guides", "How to do one thing, step by step."},
	{"concepts", "Concepts", "How Anetos works, and why."},
	{"reference", "Reference", "Settings, commands, tags and rules, to look things up."},
	{"upgrade", "Upgrading", ""},
}

const repo = "https://github.com/anetos-dev/anetos"

type page struct {
	rel    string // path in docs/site ("guides/forms.md"); "" if generated
	out    string // path in content ("guides/forms.md", "upgrade/_index.md")
	front  []string
	title  string
	intro  string
	body   []byte
	group  string // front matter's group, "" for none
	weight int    // front matter's weight, 0 for none
}

func main() {
	src := flag.String("src", "", "the framework's docs/site folder")
	out := flag.String("out", "content", "the content folder to write (replaced)")
	files := flag.String("files", "files", "the folder for the other files, served as they are (replaced)")
	overlay := flag.String("overlay", "overlay", "files copied over the result")
	data := flag.String("data", "data", "the data folder, for moved.json (the file is replaced)")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "usage: sync -src <anetos>/docs/site [-out content] [-files files] [-overlay overlay]")
		os.Exit(2)
	}
	if err := run(*src, *out, *files, *overlay, *data); err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		os.Exit(1)
	}
}

func run(src, out, files, overlay, data string) error {
	for _, dir := range []string{out, files} {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	var pages []*page
	others := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		rel = filepath.ToSlash(rel)
		if filepath.Ext(p) != ".md" {
			others++
			return copyFile(p, filepath.Join(files, filepath.FromSlash(rel)))
		}
		if rel == "README.md" {
			return nil // about the source folder; the home page is in overlay/
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		data = bytes.TrimPrefix(data, []byte("\uFEFF"))
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		pg, err := parse(rel, data)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		pages = append(pages, pg)
		return nil
	})
	if err != nil {
		return err
	}
	pages, moved, err := group(pages)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(moved, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(data, "moved.json"), append(b, '\n')); err != nil {
		return err
	}
	have := map[string]bool{}
	for _, pg := range pages {
		have[pg.out] = true
	}
	for _, s := range sections {
		index := s.dir + "/_index.md"
		if !have[index] {
			pages = append(pages, generatedIndex(s.dir, s.title, s.intro, pages, have))
			have[index] = true
		}
	}
	for i, s := range sections {
		for _, pg := range pages {
			if pg.out == s.dir+"/_index.md" {
				pg.set("weight", fmt.Sprint(i+1))
			}
		}
	}
	for _, pg := range pages {
		if w, ok := upgradeWeight(pg.rel); ok {
			pg.set("weight", fmt.Sprint(w)) // the newest version first
		}
		if pg.rel != "" {
			pg.set("sourcePath", "docs/site/"+pg.rel)
			pg.set("editURL", repo+"/edit/main/docs/site/"+pg.rel)
		}
		if pg.intro != "" {
			pg.set("description", fmt.Sprintf("%q", plain(pg.intro)))
		}
		var b bytes.Buffer
		b.WriteString("---\n")
		for _, l := range pg.front {
			b.WriteString(l + "\n")
		}
		b.WriteString("---\n\n")
		b.Write(pg.body)
		if err := writeFile(filepath.Join(out, filepath.FromSlash(pg.out)), b.Bytes()); err != nil {
			return err
		}
	}
	err = filepath.WalkDir(overlay, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(overlay, p)
		return copyFile(p, filepath.Join(out, rel))
	})
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if err == nil {
		fmt.Printf("sync: %d pages and %d other files from %s\n", len(pages), others, src)
	}
	return err
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFile(dst, data)
}

func writeFile(dst string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// set adds "key: value" to the front matter unless the page sets key.
func (pg *page) set(key, value string) {
	for _, l := range pg.front {
		if strings.HasPrefix(l, key+":") {
			return
		}
	}
	pg.front = append(pg.front, key+": "+value)
}

// parse reads a page: its front matter lines, its title (front matter's,
// else the first heading), the first paragraph after the heading, and
// the body without the heading.
func parse(rel string, data []byte) (*page, error) {
	pg := &page{rel: rel, out: rel}
	if path.Base(rel) == "README.md" {
		pg.out = path.Join(path.Dir(rel), "_index.md")
	}
	rest := data
	if bytes.HasPrefix(rest, []byte("---\n")) {
		end := bytes.Index(rest[4:], []byte("\n---\n"))
		if end < 0 {
			return nil, errors.New("front matter without its closing ---")
		}
		for _, l := range strings.Split(string(rest[4:4+end]), "\n") {
			if strings.HasPrefix(l, "title:") {
				pg.title = strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "title:")), `"'`)
			}
			if v, ok := strings.CutPrefix(l, "group:"); ok {
				pg.group = strings.Trim(strings.TrimSpace(v), `"'`)
				continue // the folder says it
			}
			if v, ok := strings.CutPrefix(l, "weight:"); ok {
				pg.weight, _ = strconv.Atoi(strings.TrimSpace(v))
			}
			pg.front = append(pg.front, l)
		}
		rest = rest[4+end+5:]
	}
	rest = bytes.TrimLeft(rest, "\n")
	if bytes.HasPrefix(rest, []byte("# ")) {
		nl := bytes.IndexByte(rest, '\n')
		if nl < 0 {
			nl = len(rest)
		}
		heading := strings.TrimSpace(string(rest[2:nl]))
		if pg.title == "" {
			pg.title = heading
			pg.set("title", fmt.Sprintf("%q", heading))
		}
		rest = bytes.TrimLeft(rest[nl:], "\n")
	}
	if pg.title == "" {
		return nil, errors.New("no title: give it front matter or a # heading")
	}
	pg.body = rest
	// the first paragraph, for section indexes and the description
	sc := bufio.NewScanner(bytes.NewReader(rest))
	var para []string
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "```") || strings.HasPrefix(l, "|") ||
			strings.HasPrefix(l, "-") || strings.HasPrefix(l, ">") || strings.HasPrefix(l, "<") ||
			strings.HasPrefix(l, "{{") {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, l)
	}
	pg.intro = strings.Join(para, " ")
	return pg, nil
}

// generatedIndex is the section page of dir: its intro and a list of its
// pages and subsections with their first paragraphs. have holds the
// pages' output paths, to tell subsections (folders with a README) from
// plain folders, whose pages belong to the section above.
func generatedIndex(dir, title, intro string, pages []*page, have map[string]bool) *page {
	// sectionOf is the section a page is listed in (for a README, the
	// section its folder is in).
	sectionOf := func(rel string) string {
		d := path.Dir(rel)
		if path.Base(rel) == "README.md" {
			d = path.Dir(d)
		}
		for d != "." && d != dir && !have[d+"/_index.md"] {
			d = path.Dir(d)
		}
		return d
	}
	var kids []*page
	for _, pg := range pages {
		if pg.rel != "" && strings.HasPrefix(pg.rel, dir+"/") && sectionOf(pg.rel) == dir {
			kids = append(kids, pg)
		}
	}
	slices.SortFunc(kids, func(a, b *page) int {
		if a.weight != b.weight {
			return a.weight - b.weight
		}
		return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
	})
	var b bytes.Buffer
	if intro != "" {
		b.WriteString(intro + "\n\n")
	}
	group := ""
	for _, k := range kids {
		if k.group != group {
			group = k.group
			fmt.Fprintf(&b, "\n## %s\n\n", group)
		}
		fmt.Fprintf(&b, "- [%s](%s)", k.title, strings.TrimPrefix(k.rel, dir+"/"))
		if intro := k.intro; intro != "" {
			if path.Dir(k.rel) != dir {
				intro = plain(intro) // its links are relative to another folder
			}
			fmt.Fprintf(&b, ": %s", intro)
		}
		b.WriteString("\n")
	}
	desc := title
	if intro != "" {
		desc = intro
	}
	return &page{
		out: dir + "/_index.md",
		front: []string{
			fmt.Sprintf("title: %q", title),
			// relative links in the list resolve against this
			"sourcePath: docs/site/" + dir + "/README.md",
			// the intro is in this program; the list is the section's pages
			"editURL: https://github.com/anetos-dev/docs/edit/main/sync/main.go",
			fmt.Sprintf("description: %q", desc),
		},
		title: title,
		body:  b.Bytes(),
	}
}

var (
	mdLink  = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdMarks = strings.NewReplacer("`", "", "**", "", "__", "")
)

// plain is Markdown text without its links and marks, for descriptions.
func plain(md string) string {
	return mdMarks.Replace(mdLink.ReplaceAllString(md, "$1"))
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// slug is a group's folder: "Accounts and security" →
// accounts-and-security. The framework's docnav makes the same.
func slug(group string) string {
	return strings.Trim(nonWord.ReplaceAllString(strings.ToLower(group), "-"), "-")
}

// group moves the pages that name a group into a folder of their
// section for it, keeping their URL, and adds each folder's index. It
// returns the pages, and their old content paths (without .md) mapped
// to the new ones, for the link render hook.
func group(pages []*page) ([]*page, map[string]string, error) {
	moved := map[string]string{}
	type folder struct {
		dir, title string
		weight     int
		pages      []*page
	}
	folders := map[string]*folder{}
	var order []string
	names := map[string]bool{} // the sections' pages and folders, by URL path
	for _, pg := range pages {
		names[strings.TrimSuffix(pg.out, ".md")] = true
		for d := path.Dir(pg.rel); d != "."; d = path.Dir(d) {
			names[d] = true
		}
	}
	for _, pg := range pages {
		if pg.group == "" {
			continue
		}
		dir := path.Dir(pg.rel)
		if path.Base(pg.rel) == "README.md" || strings.Contains(dir, "/") {
			return nil, nil, fmt.Errorf("%s: only the pages directly in a section have a group", pg.rel)
		}
		key := dir + "/" + slug(pg.group)
		if names[key] {
			return nil, nil, fmt.Errorf("%s: group %q would take the URL of %s", pg.rel, pg.group, key)
		}
		f := folders[key]
		if f == nil {
			f = &folder{dir: key, title: pg.group, weight: pg.weight}
			folders[key] = f
			order = append(order, key)
		}
		f.weight = min(f.weight, pg.weight)
		f.pages = append(f.pages, pg)
		name := strings.TrimSuffix(path.Base(pg.rel), ".md")
		old := strings.TrimSuffix(pg.out, ".md")
		pg.out = key + "/" + name + ".md"
		moved["/"+old] = "/" + strings.TrimSuffix(pg.out, ".md")
		pg.set("url", "/"+dir+"/"+name+"/")
	}
	for _, key := range order {
		f := folders[key]
		slices.SortFunc(f.pages, func(a, b *page) int { return a.weight - b.weight })
		var b bytes.Buffer
		for _, k := range f.pages {
			fmt.Fprintf(&b, "- [%s](%s)", k.title, path.Base(k.rel))
			if k.intro != "" {
				fmt.Fprintf(&b, ": %s", k.intro)
			}
			b.WriteString("\n")
		}
		section := path.Dir(key)
		pages = append(pages, &page{
			out: key + "/_index.md",
			front: []string{
				fmt.Sprintf("title: %q", f.title),
				fmt.Sprintf("weight: %d", f.weight),
				// the list's links are relative to the section's folder
				"sourcePath: docs/site/" + section + "/README.md",
				"editURL: https://github.com/anetos-dev/docs/edit/main/sync/main.go",
				fmt.Sprintf("description: %q", f.title),
			},
			title:  f.title,
			body:   b.Bytes(),
			weight: f.weight,
		})
	}
	return pages, moved, nil
}

var upgradePage = regexp.MustCompile(`^upgrade/v(\d+)\.(\d+)(?:\.(\d+))?\.md$`)

// upgradeWeight is the weight of an upgrade guide, which puts the newest
// version first: v1.2 before v1.1 before v0.9.
func upgradeWeight(rel string) (int, bool) {
	m := upgradePage.FindStringSubmatch(rel)
	if m == nil {
		return 0, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return 1_000_000_000 - (major*1_000_000 + minor*1_000 + patch), true
}
