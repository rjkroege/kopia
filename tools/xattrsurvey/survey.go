package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kopia/kopia/fs"
)

const (
	histogramBuckets = 24
	maxExamples      = 5

	finderInfoSize       = 32
	finderFlagsOffset    = 8
	kHasBundle           = 0x2000
	exampleValueSize     = 64 << 10
	filesecEntriesOffset = 36

	// JSON overhead of "name":"value" and of an object ID reference.
	jsonPairOverhead = 6
	objectIDLength   = 65
)

// thresholds evaluated for a hybrid inline/object format; 0 means all objects.
var thresholds = []int{0, 64, 256, 1 << 10, 4 << 10, 16 << 10, 64 << 10, 1 << 30} //nolint:gochecknoglobals

var bundleExtensions = map[string]bool{ //nolint:gochecknoglobals
	".app": true, ".appex": true, ".band": true, ".bundle": true, ".dSYM": true,
	".fcpbundle": true, ".framework": true, ".imovielibrary": true, ".kext": true, ".key": true,
	".logicx": true, ".mlmodelc": true, ".mpkg": true, ".musiclibrary": true, ".numbers": true,
	".pages": true, ".photoslibrary": true, ".pkg": true, ".playground": true, ".plugin": true,
	".prefPane": true, ".pvm": true, ".qlgenerator": true, ".rtfd": true, ".saver": true,
	".scptd": true, ".sparsebundle": true, ".utm": true, ".vmwarevm": true, ".wdgt": true,
	".workflow": true, ".xcodeproj": true, ".xcworkspace": true, ".xpc": true,
}

type sizeHist struct {
	count [histogramBuckets]int64
	bytes [histogramBuckets]int64
}

// bucket returns the log2 bucket: 0 for 0, 1 for 1, 2 for 2-3, 3 for 4-7, ...
func bucket(n int) int {
	return min(bits.Len(uint(n)), histogramBuckets-1)
}

func (h *sizeHist) add(n int) {
	b := bucket(n)
	h.count[b]++
	h.bytes[b] += int64(n)
}

type nameStat struct {
	count, bytes, maxSize int64
}

// survey accumulates statistics; all fields are guarded by mu.
type survey struct {
	mu sync.Mutex

	entries, files, dirs, symlinks int64
	withXattr, xattrCount          int64
	xattrBytes                     int64
	errors                         map[string]int64

	valueSize     sizeHist
	entryTotal    sizeHist
	perEntryCount map[int]int64
	names         map[string]*nameStat
	values        map[[32]byte]int // hash of name and value -> value size

	inlineTotal   []int64
	objectsTotal  []int64
	dirInlineMax  []int64
	dirInlineHist []sizeHist

	flagBits    map[uint32]int64
	withFlags   int64
	withBirth   int64
	birthDiffer int64
	withACL     int64
	aclEntries  int64
	aclSizeHist sizeHist

	finderFlags   map[uint16]int64
	bundleBitDirs int64
	bundleExtDirs map[string]int64
	bundleExtBit  map[string][2]int64 // [with kHasBundle, without]
	examples      map[string][]string
}

func newSurvey() *survey {
	n := len(thresholds)

	return &survey{
		errors:        map[string]int64{},
		perEntryCount: map[int]int64{},
		names:         map[string]*nameStat{},
		values:        map[[32]byte]int{},
		inlineTotal:   make([]int64, n),
		objectsTotal:  make([]int64, n),
		dirInlineMax:  make([]int64, n),
		dirInlineHist: make([]sizeHist, n),
		flagBits:      map[uint32]int64{},
		finderFlags:   map[uint16]int64{},
		bundleExtDirs: map[string]int64{},
		bundleExtBit:  map[string][2]int64{},
		examples:      map[string][]string{},
	}
}

func (s *survey) example(kind, path string) {
	if len(s.examples[kind]) < maxExamples {
		s.examples[kind] = append(s.examples[kind], path)
	}
}

func (s *survey) countError(kind string, err error) {
	msg := err.Error()
	if _, after, ok := strings.Cut(msg, ": "); ok {
		msg = after
	}

	s.mu.Lock()
	s.errors[kind+": "+msg]++
	s.mu.Unlock()
}

// inlineCost estimates the JSON bytes an inlined xattr adds to a manifest (base64 value).
func inlineCost(name string, size int) int64 {
	return int64(len(name) + jsonPairOverhead + (size+2)/3*4) //nolint:mnd
}

// objectRefCost estimates the JSON bytes of a reference to an object holding the value.
func objectRefCost(name string) int64 {
	return int64(len(name) + jsonPairOverhead + objectIDLength)
}

// recordEntry records stats for one entry and returns its inline cost for each threshold.
func (s *survey) recordEntry(ctx context.Context, e fs.Entry, opts fs.AttributeOptions) []int64 {
	path := e.LocalFilesystemPath()

	attrs, err := fs.ReadExtendedAttributes(ctx, e, opts)
	if err != nil {
		s.countError("xattr", err)
	}

	pi, err := platformStat(path)
	if err != nil {
		s.countError("stat", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.recordType(e)
	s.recordPlatform(path, pi)

	if attrs == nil {
		return make([]int64, len(thresholds))
	}

	s.recordACL(path, attrs.ACL)

	return s.recordXattrs(e, path, attrs.Xattrs)
}

func (s *survey) recordType(e fs.Entry) {
	s.entries++

	switch {
	case e.IsDir():
		s.dirs++

		if ext := filepath.Ext(e.Name()); bundleExtensions[ext] {
			s.bundleExtDirs[ext]++
		}

	case e.Mode()&os.ModeSymlink != 0:
		s.symlinks++

	default:
		s.files++
	}
}

func (s *survey) recordPlatform(path string, pi platformInfo) {
	if pi.flags != 0 {
		s.withFlags++

		for b := range 32 {
			if pi.flags&(1<<b) != 0 {
				s.flagBits[1<<b]++
			}
		}

		if pi.flags&ufHidden != 0 {
			s.example("UF_HIDDEN", path)
		}
	}

	if pi.hasBirth {
		s.withBirth++
	}

	if pi.birthDiffers {
		s.birthDiffer++
	}
}

func (s *survey) recordACL(path string, filesec []byte) {
	if len(filesec) < filesecEntriesOffset+4 {
		return
	}

	s.withACL++
	s.aclEntries += int64(binary.NativeEndian.Uint32(filesec[filesecEntriesOffset:]))
	s.aclSizeHist.add(len(filesec))
	s.example("ACL", path)
}

func (s *survey) recordXattrs(e fs.Entry, path string, xattrs map[string][]byte) []int64 {
	cost := make([]int64, len(thresholds))

	if len(xattrs) == 0 {
		return cost
	}

	s.withXattr++
	s.perEntryCount[len(xattrs)]++

	total := 0

	for name, v := range xattrs {
		total += len(v)
		s.recordValue(name, v)

		for i, t := range thresholds {
			if len(v) <= t {
				cost[i] += inlineCost(name, len(v))
			} else {
				cost[i] += objectRefCost(name)
				s.objectsTotal[i]++
			}
		}

		if len(v) > exampleValueSize {
			s.example("value>64KiB "+name, path)
		}

		if name == "com.apple.FinderInfo" {
			s.recordFinderInfo(e, path, v)
		}
	}

	s.entryTotal.add(total)

	for i := range cost {
		s.inlineTotal[i] += cost[i]
	}

	return cost
}

func (s *survey) recordValue(name string, v []byte) {
	s.xattrCount++
	s.xattrBytes += int64(len(v))
	s.valueSize.add(len(v))

	ns := s.names[name]
	if ns == nil {
		ns = &nameStat{}
		s.names[name] = ns
	}

	ns.count++
	ns.bytes += int64(len(v))
	ns.maxSize = max(ns.maxSize, int64(len(v)))

	s.values[sha256.Sum256(append([]byte(name+"\x00"), v...))] = len(v)
}

func (s *survey) recordFinderInfo(e fs.Entry, path string, v []byte) {
	if len(v) < finderInfoSize {
		return
	}

	ff := binary.BigEndian.Uint16(v[finderFlagsOffset:])
	for b := range 16 {
		if ff&(1<<b) != 0 {
			s.finderFlags[1<<b]++
		}
	}

	if !e.IsDir() {
		return
	}

	ext := filepath.Ext(e.Name())
	bb := s.bundleExtBit[ext]

	if ff&kHasBundle != 0 {
		s.bundleBitDirs++
		bb[0]++

		s.example("kHasBundle dir", path)
	} else {
		bb[1]++
	}

	s.bundleExtBit[ext] = bb
}

func (s *survey) recordDirectoryCost(dirCost []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range dirCost {
		if c > 0 {
			s.dirInlineHist[i].add(int(c))
		}

		s.dirInlineMax[i] = max(s.dirInlineMax[i], c)
	}
}

type jsonSummary struct {
	Root    string                  `json:"root"`
	Entries int64                   `json:"entries"`
	Xattrs  int64                   `json:"xattrs"`
	Bytes   int64                   `json:"bytes"`
	Counts  [histogramBuckets]int64 `json:"valueSizeLog2Counts"`
	Sizes   [histogramBuckets]int64 `json:"valueSizeLog2Bytes"`
	Names   map[string][3]int64     `json:"names"` // count, bytes, max size
}

func (s *survey) jsonSummary(root string) jsonSummary {
	j := jsonSummary{
		Root: root, Entries: s.entries, Xattrs: s.xattrCount, Bytes: s.xattrBytes,
		Counts: s.valueSize.count, Sizes: s.valueSize.bytes, Names: map[string][3]int64{},
	}

	for n, v := range s.names {
		j.Names[n] = [3]int64{v.count, v.bytes, v.maxSize}
	}

	return j
}
