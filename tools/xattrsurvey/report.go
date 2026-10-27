package main

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	barWidth       = 40
	topNamesCount  = 40
	topBytesCount  = 10
	dirPercentile  = 0.99
	bytesPerKibi   = 1024
	tabPadding     = 2
	allObjectsSize = 0
	allInlineSize  = 1 << 30
)

// printer writes formatted text and remembers the first error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

// table returns a printer for an aligned table; call flush when done.
func (p *printer) table(flags uint) (*printer, func()) {
	tw := tabwriter.NewWriter(p.w, 0, 0, tabPadding, ' ', flags)
	t := &printer{w: tw}

	return t, func() {
		if t.err == nil {
			t.err = tw.Flush()
		}

		if p.err == nil {
			p.err = t.err
		}
	}
}

func human(n int64) string {
	if n < bytesPerKibi {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(bytesPerKibi), 0
	for m := n / bytesPerKibi; m >= bytesPerKibi; m /= bytesPerKibi {
		div *= bytesPerKibi
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func bucketLabel(b int) string {
	if b <= 1 {
		return strconv.Itoa(b)
	}

	return human(int64(1)<<(b-1)) + "-" + human(int64(1)<<b-1)
}

func printHist(p *printer, title string, h *sizeHist) {
	var tc, tb, peak int64

	for i := range h.count {
		tc += h.count[i]
		tb += h.bytes[i]
		peak = max(peak, h.count[i])
	}

	p.printf("\n## %s  (n=%d, total=%s)\n", title, tc, human(tb))

	if tc == 0 {
		return
	}

	t, flush := p.table(tabwriter.AlignRight)
	defer flush()

	t.printf("size\tcount\tcum%%\tbytes\tbytes%%\t\n")

	var cum int64

	for i := range h.count {
		if h.count[i] == 0 {
			continue
		}

		cum += h.count[i]
		bar := strings.Repeat("#", int((h.count[i]*barWidth+peak-1)/peak))
		t.printf("%s\t%d\t%.1f\t%s\t%.1f\t %s\n", bucketLabel(i), h.count[i],
			100*float64(cum)/float64(tc), human(h.bytes[i]), 100*float64(h.bytes[i])/float64(max(tb, 1)), bar)
	}
}

func percentile(h *sizeHist, q float64) int64 {
	var tc, cum int64

	for _, c := range h.count {
		tc += c
	}

	for i, c := range h.count {
		cum += c
		if c > 0 && float64(cum) >= q*float64(tc) {
			return max(int64(1)<<i-1, 0)
		}
	}

	return 0
}

func (s *survey) report(w io.Writer, root string, elapsed time.Duration) error {
	p := &printer{w: w}

	s.reportSummary(p, root, elapsed)
	printHist(p, "xattr value size", &s.valueSize)
	printHist(p, "total xattr bytes per entry (entries with xattrs)", &s.entryTotal)
	s.reportNames(p)
	s.reportFormats(p)
	s.reportPlatform(p)
	s.reportBundles(p)
	s.reportErrorsAndExamples(p)

	return p.err
}

func (s *survey) reportSummary(p *printer, root string, elapsed time.Duration) {
	p.printf("# xattr survey of %s (%v)\n\n", root, elapsed.Round(time.Second))
	p.printf("entries=%d files=%d dirs=%d symlinks=%d\n", s.entries, s.files, s.dirs, s.symlinks)
	p.printf("entries with xattrs: %d (%.2f%%), xattrs=%d, value bytes=%s\n",
		s.withXattr, 100*float64(s.withXattr)/float64(max(s.entries, 1)), s.xattrCount, human(s.xattrBytes))

	var uniqBytes int64
	for _, size := range s.values {
		uniqBytes += int64(size)
	}

	p.printf("unique (name,value) pairs: %d of %d; unique value bytes=%s (dedup ratio %.2fx)\n",
		len(s.values), s.xattrCount, human(uniqBytes), float64(s.xattrBytes)/float64(max(uniqBytes, 1)))
}

func (s *survey) reportNames(p *printer) {
	p.printf("\n## xattrs per entry\n")

	for _, k := range slices.Sorted(maps.Keys(s.perEntryCount)) {
		p.printf("  %3d: %d\n", k, s.perEntryCount[k])
	}

	names := slices.Collect(maps.Keys(s.names))

	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(s.names[b].count, s.names[a].count), cmp.Compare(s.names[b].bytes, s.names[a].bytes))
	})

	p.printf("\n## top xattr names (by count, then bytes)\n")
	s.printNames(p, names, topNamesCount)

	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(s.names[b].bytes, s.names[a].bytes) })

	p.printf("\n## top xattr names by bytes\n")
	s.printNames(p, names, topBytesCount)
}

func (s *survey) printNames(p *printer, names []string, limit int) {
	t, flush := p.table(0)
	defer flush()

	t.printf("  name\tcount\tbytes\tmax\n")

	for i, n := range names {
		if i >= limit {
			t.printf("  ... %d more names\t\t\t\n", len(names)-i)
			return
		}

		t.printf("  %s\t%d\t%s\t%s\n", n, s.names[n].count, human(s.names[n].bytes), human(s.names[n].maxSize))
	}
}

func (s *survey) reportFormats(p *printer) {
	p.printf("\n## storage format: inline below threshold, object above\n")

	t, flush := p.table(tabwriter.AlignRight)
	defer flush()

	t.printf("threshold\tmanifest bytes added\tobjects\tmax per-dir manifest add\tp99 dir add\t\n")

	for i, th := range thresholds {
		label := human(int64(th))

		switch th {
		case allObjectsSize:
			label = "all objects"
		case allInlineSize:
			label = "all inline"
		}

		t.printf("%s\t%s\t%d\t%s\t%s\t\n", label, human(s.inlineTotal[i]), s.objectsTotal[i],
			human(s.dirInlineMax[i]), human(percentile(&s.dirInlineHist[i], dirPercentile)))
	}
}

func (s *survey) reportPlatform(p *printer) {
	p.printf("\n## BSD file flags (st_flags): %d entries have any\n", s.withFlags)

	for _, b := range slices.Sorted(maps.Keys(s.flagBits)) {
		p.printf("  0x%08x %-14s %d\n", b, bsdFlagNames[b], s.flagBits[b])
	}

	p.printf("\n## birth time: %d entries have it, %d differ from mtime\n", s.withBirth, s.birthDiffer)
	p.printf("\n## extended ACLs: %d entries, %d ACEs\n", s.withACL, s.aclEntries)

	if s.withACL > 0 {
		printHist(p, "ACL (kauth_filesec) size", &s.aclSizeHist)
	}

	p.printf("\n## FinderInfo finderFlags bits\n")

	for _, b := range slices.Sorted(maps.Keys(s.finderFlags)) {
		p.printf("  0x%04x %-16s %d\n", b, finderFlagNames[b], s.finderFlags[b])
	}
}

func (s *survey) reportBundles(p *printer) {
	p.printf("\n## bundles: %d dirs carry kHasBundle\n", s.bundleBitDirs)

	for _, ext := range slices.Sorted(maps.Keys(s.bundleExtDirs)) {
		bb := s.bundleExtBit[ext]
		p.printf("  %-16s dirs=%-7d FinderInfo w/ kHasBundle=%d w/o=%d\n", ext, s.bundleExtDirs[ext], bb[0], bb[1])
	}

	for _, ext := range slices.Sorted(maps.Keys(s.bundleExtBit)) {
		if bb := s.bundleExtBit[ext]; s.bundleExtDirs[ext] == 0 && bb[0] > 0 {
			p.printf("  %-16q (no known ext) kHasBundle=%d\n", ext, bb[0])
		}
	}
}

func (s *survey) reportErrorsAndExamples(p *printer) {
	p.printf("\n## errors\n")

	for _, k := range slices.Sorted(maps.Keys(s.errors)) {
		p.printf("  %-50s %d\n", k, s.errors[k])
	}

	p.printf("\n## examples\n")

	for _, k := range slices.Sorted(maps.Keys(s.examples)) {
		p.printf("  %s:\n", k)

		for _, path := range s.examples[k] {
			p.printf("    %s\n", path)
		}
	}
}

var finderFlagNames = map[uint16]string{ //nolint:gochecknoglobals
	0x0001: "kIsOnDesk", 0x0002: "color0", 0x0004: "color1", 0x0008: "color2",
	0x0010: "kExtensionHidden", 0x0040: "kIsShared", 0x0080: "kHasNoINITs",
	0x0100: "kHasBeenInited", 0x0400: "kHasCustomIcon", 0x0800: "kIsStationery",
	0x1000: "kNameLocked", 0x2000: "kHasBundle", 0x4000: "kIsInvisible", 0x8000: "kIsAlias",
}
