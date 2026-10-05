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
//   - Other files (images…) are copied to the -files folder, which Hugo
//     serves at their path in docs/site.
//   - The files in overlay/ (the docs' home page) are copied over the top.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
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
	rel   string // path in docs/site ("guides/forms.md"); "" if generated
	out   string // path in content ("guides/forms.md", "upgrade/_index.md")
	front []string
	title string
	intro string
	body  []byte
}

func main() {
	src := flag.String("src", "", "the framework's docs/site folder")
	out := flag.String("out", "content", "the content folder to write (replaced)")
	files := flag.String("files", "files", "the folder for the other files, served as they are (replaced)")
	overlay := flag.String("overlay", "overlay", "files copied over the result")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "usage: sync -src <anetos>/docs/site [-out content] [-files files] [-overlay overlay]")
		os.Exit(2)
	}
	if err := run(*src, *out, *files, *overlay); err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		os.Exit(1)
	}
}

func run(src, out, files, overlay string) error {
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
	slices.SortFunc(kids, func(a, b *page) int { return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title)) })
	var b bytes.Buffer
	if intro != "" {
		b.WriteString(intro + "\n\n")
	}
	for _, k := range kids {
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
