// Command xattrsurvey walks a directory tree the way "kopia snapshot create" would (honoring
// .kopiaignore and the default policy's ignore rules) and reports histograms of extended
// attribute sizes and names, BSD file flags, ACLs, birth times and bundle markers. It is a
// research aid for sizing extended metadata in directory manifests; it does not touch any
// repository.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/fs/ignorefs"
	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/internal/timetrack"
	"github.com/kopia/kopia/snapshot/policy"
)

const (
	defaultParallelism = 16
	progressInterval   = 10 * time.Second
	jsonFileMode       = 0o600
)

type walker struct {
	s    *survey
	opts fs.AttributeOptions
	sem  chan struct{}
	wg   sync.WaitGroup
}

func (w *walker) walkDir(ctx context.Context, d fs.Directory) {
	dirCost := make([]int64, len(thresholds))

	err := fs.IterateEntries(ctx, d, func(ctx context.Context, e fs.Entry) error {
		for i, c := range w.s.recordEntry(ctx, e, w.opts) {
			dirCost[i] += c
		}

		sub, ok := e.(fs.Directory)
		if !ok {
			return nil
		}

		select {
		case w.sem <- struct{}{}:
			w.wg.Go(func() {
				defer func() { <-w.sem }()

				w.walkDir(ctx, sub)
			})
		default:
			w.walkDir(ctx, sub)
		}

		return nil
	})
	if err != nil {
		w.s.countError("readdir", err)
	}

	w.s.recordDirectoryCost(dirCost)
}

func reportProgress(s *survey, done <-chan struct{}) {
	t := time.NewTicker(progressInterval)
	defer t.Stop()

	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.mu.Lock()
			fmt.Fprintf(os.Stderr, "... %d entries, %d with xattrs\n", s.entries, s.withXattr)
			s.mu.Unlock()
		}
	}
}

func run(ctx context.Context, root string, oneFS bool, parallel int, exclude, jsonPath string) error {
	dir, err := localfs.Directory(root)
	if err != nil {
		return errors.Wrap(err, "unable to open root")
	}

	pol := *policy.DefaultPolicy
	if oneFS {
		pol.FilesPolicy.OneFileSystem = policy.NewOptionalBool(true)
	}

	// The policy must be defined at the root, not inherited, for ignore files to apply.
	walked := ignorefs.New(dir, policy.BuildTree(map[string]*policy.Policy{".": &pol}, &pol))

	var excludes []string

	for n := range strings.SplitSeq(exclude, ",") {
		if n != "" {
			excludes = append(excludes, n)
		}
	}

	s := newSurvey()
	w := &walker{
		s: s,
		opts: fs.AttributeOptions{
			Xattrs:        fs.XattrsAll,
			ExcludeXattrs: excludes,
			MaxXattrSize:  int(^uint(0) >> 1),
			ACLs:          true,
		},
		sem: make(chan struct{}, parallel),
	}

	timer := timetrack.StartTimer()
	done := make(chan struct{})

	go reportProgress(s, done)

	w.walkDir(ctx, walked)
	w.wg.Wait()
	close(done)

	if err := s.report(os.Stdout, root, timer.Elapsed()); err != nil {
		return errors.Wrap(err, "unable to write report")
	}

	if jsonPath == "" {
		return nil
	}

	b, err := json.MarshalIndent(s.jsonSummary(root), "", " ")
	if err != nil {
		return errors.Wrap(err, "unable to marshal JSON")
	}

	return errors.Wrap(os.WriteFile(jsonPath, b, jsonFileMode), "unable to write JSON") //nolint:gosec // the user chooses the output path
}

func main() {
	root := flag.String("root", os.Getenv("HOME"), "directory to survey")
	oneFS := flag.Bool("one-file-system", false, "do not cross filesystem boundaries")
	parallel := flag.Int("parallel", defaultParallelism, "directory walkers")
	jsonPath := flag.String("json", "", "also write raw histogram JSON here")
	exclude := flag.String("exclude", "", "comma-separated xattr names to skip (a trailing '*' matches a prefix)")

	flag.Parse()

	if err := run(context.Background(), *root, *oneFS, *parallel, *exclude, *jsonPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
