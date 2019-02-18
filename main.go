// Command info displays texinfo pages.
package main // import "arp242.net/info"

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	isatty "github.com/mattn/go-isatty"
)

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("which page?"))
	}

	fp := find(os.Args[1])
	if fp == nil {
		fatal(fmt.Errorf("no page for %q", os.Args[1]))
	}
	defer fp.Close()

	page := format(fp)

	if isatty.IsTerminal(os.Stdout.Fd()) {
		err := pager(page)
		fatal(err)
		return
	}

	fmt.Println(page)
}

func fatal(err error) {
	if err == nil {
		return
	}

	_, _ = fmt.Fprintf(os.Stderr, "info: %v\n", err)
	os.Exit(1)

}

func infopath() []string {
	var dirs []string
	if os.Getenv("INFOPATH") != "" {
		for _, p := range strings.Split(":", os.Getenv("INFOPATH")) {
			if p != "" {
				dirs = append(dirs, p)
			}
		}
	}
	if len(dirs) > 0 {
		return dirs
	}

	return []string{"/usr/share/info/"}
}

func find(page string) io.ReadCloser {
	// So that includes in the form "tar.info-1" work.
	addext := ""
	if !strings.Contains(page, ".info") {
		addext = ".info"
	}

	var (
		fp  io.ReadCloser
		err error
	)
	for _, d := range infopath() {
		fp, err = os.Open(fmt.Sprintf("%s/%s%s", d, page, addext))
		if os.IsNotExist(err) {
			fp, err = os.Open(fmt.Sprintf("%s/%s%s.gz", d, page, addext))
			if err == nil {
				fp, err = gzip.NewReader(fp)
			}
		}
		if os.IsNotExist(err) {
			continue
		}

		if err != nil {
			fatal(err)
		}
		return fp
	}

	return nil
}

var (
	sub = []*regexp.Regexp{
		regexp.MustCompile(`(^|\n)File: [\w\-]+.info,  Node: .+($|\n)`), // Navigation
		regexp.MustCompile(`\n\* Menu:\n\n(\* .+?::( .+?)?\n){1,}`),     // Menu for subtree
		regexp.MustCompile(`\n{3,}`),
	}
	repl = []string{"", "", "\n\n"}

	nuke = []*regexp.Regexp{
		// Often longer than manpage 🤦
		regexp.MustCompile(`^\s*\d+ Copying\n\*{9,}\n\n`),
		regexp.MustCompile(`^\s*[\d.]+ GNU Free Documentation License\n={32,}\n\n`),
		regexp.MustCompile(`Appendix \w Free Software Needs Free Documentation\n\*{49,}\n\n`),
		regexp.MustCompile(`Appendix \w GNU Free Documentation License\n\*{41,}\n\n`),
		regexp.MustCompile(`GNU General Public License\n\*{26}`),

		// Free software bruhaha repeated ad nauseam.
		regexp.MustCompile(`Permission is granted to copy, distribute and/or modify this`),

		regexp.MustCompile(`\x00\x08\[index\x00\x08]\n`),
		regexp.MustCompile(`^\s*Tag Table:\n`),
		regexp.MustCompile(`^\s*End Tag Table\n`),
		regexp.MustCompile(`^\s*Local Variables:\n`),

		regexp.MustCompile(`\nIndirect:\n(.+?: \d+\n)+?`),
	}

	// ^_
	// Indirect:
	// tar.info-1: 1139
	// tar.info-2: 303202
	// ^_
	reInclude = regexp.MustCompile(`\x1f\nIndirect:\n(.+?: \d+\n)+?\x1f`)
)

func format(fp io.Reader) string {
	d, err := ioutil.ReadAll(fp)
	fatal(err)

	// Load subpages.
	var subpages string
	if m := reInclude.Find(d); m != nil {
		for _, f := range bytes.Split(m, []byte("\n"))[2:] {
			path := bytes.Split(f, []byte(":"))
			if len(path) != 2 {
				continue
			}

			subfp := find(string(path[0]))
			if subfp == nil {
				fatal(fmt.Errorf("could not find included page %q", path[0]))
			}

			subpages += format(subfp) + "\n\n"
			_ = subfp.Close()
		}
	}

	pages := strings.Split(string(d), "\x1f")
	for i := range pages {
		for j, re := range sub {
			pages[i] = re.ReplaceAllString(pages[i], repl[j])
		}

		for _, re := range nuke {
			if re.MatchString(pages[i]) {
				pages[i] = ""
			}
		}

		pages[i] = strings.TrimSpace(pages[i])
	}

	return strings.TrimSpace(strings.Join(pages, "\n\n")+subpages) + "\n"
}

func pager(page string) error {
	tmp, err := ioutil.TempFile(os.TempDir(), "info.")
	if err != nil {
		return err
	}

	_, err = tmp.WriteString(page)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	err = tmp.Close()
	if err != nil {
		return err
	}

	defer os.Remove(tmp.Name())

	p := os.Getenv("MANPAGER")
	if p == "" {
		p = os.Getenv("PAGER")
	}
	if p == "" {
		p = "more -s"
	}

	cmd := exec.Command("/bin/sh", "-c", p+" "+tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
