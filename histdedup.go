package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mattn/go-shellwords"
)

var (
	rxTime  = regexp.MustCompile(`^#(\d+)$`)
	bStdout bool
)

func main() {

	var E error
	defer func() {
		if E != nil {
			fmt.Fprintf(os.Stderr, "\x1b[91mERR\x1b[0m | %s\n", E.Error())
			os.Exit(1)
		}
	}()

	flag.BoolVar(&bStdout, "stdout", false, "write to STDOUT instead of overwriting source file")
	flag.Parse()

	for _, fname := range flag.Args() {
		if E = dedupeFile(fname); E != nil {
			return
		}
	}
}

func dedupeFile(srcfile string) (E error) {

	defer func() {
		if E != nil {
			E = fmt.Errorf("%s | %w", srcfile, E)
		}
	}()

	// open source
	var bsSrc []byte
	bsSrc, E = os.ReadFile(srcfile)
	if E != nil {
		return
	}

	// dedupe
	bufwri := bytes.NewBuffer(make([]byte, 0, len(bsSrc)))
	E = dedupeStream(bufwri, bytes.NewReader(bsSrc))
	if E != nil {
		return
	}

	// write result
	if bStdout {
		_, E = os.Stdout.Write(bufwri.Bytes())
	} else {
		E = os.WriteFile(srcfile, bufwri.Bytes(), 0600)
	}

	return
}

func dedupeStream(iW io.Writer, iRd io.Reader) (E error) {

	var nLine int
	defer func() {
		if E != nil {
			E = fmt.Errorf("line %d | %w", nLine, E)
		}
	}()

	pscan := bufio.NewScanner(iRd)
	mLine2Date := make(map[string]int64, 1024)

	var lineTstamp int64
	for pscan.Scan() {

		lineTxt := pscan.Text()
		nLine++

		// parse timestamp
		if (nLine & 1) == 1 {
			pts := rxTime.FindStringSubmatch(lineTxt)
			if len(pts) != 2 {
				E = fmt.Errorf("expected history timestamp, received %s", strconv.Quote(lineTxt))
				return
			}
			lineTstamp, _ = strconv.ParseInt(pts[1], 10, 64)
			continue
		}

		// parse command

		// shell parse to normalize
		if spts, epts := shellwords.Parse(lineTxt); epts == nil {
			lineTxt = strings.Join(spts, " ")
		} else {
			// fallback to trim if the line is not valid shell syntax
			lineTxt = strings.TrimSpace(lineTxt)
		}

		// deduplicate, track earliest timestamp
		if tprev, ok := mLine2Date[lineTxt]; ok && lineTstamp >= tprev {
			continue
		}
		mLine2Date[lineTxt] = lineTstamp
	}

	// report any line scanner errors
	if E = pscan.Err(); E != nil {
		return
	}

	// sort by timestamp ascending
	type histline struct {
		Time int64
		Line string
	}
	ret := make([]*histline, 0, len(mLine2Date))
	for ln, ts := range mLine2Date {
		ret = append(ret, &histline{Time: ts, Line: ln})
	}
	slices.SortFunc(ret, func(a, b *histline) int {
		if a.Time < b.Time {
			return -1
		} else if a.Time == b.Time {
			return 0
		}
		return 1
	})

	// report
	for _, v := range ret {
		fmt.Fprintf(iW, "#%d\n%s\n", v.Time, v.Line)
	}
	return
}
