package main

import (
	"io"
	"io/ioutil"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/kopia/kopia/blob"
	"github.com/kopia/kopia/cas"
	"github.com/kopia/kopia/fs"
)

const (
	maxWorkerCount = 3
)

type highLatencyStorage struct {
	blob.Storage

	readDelay  time.Duration
	writeDelay time.Duration
}

func (hls *highLatencyStorage) PutBlock(id blob.BlockID, data io.ReadCloser, options blob.PutOptions) error {
	go func() {
		time.Sleep(hls.writeDelay)
		hls.Storage.PutBlock(id, data, options)
	}()

	return nil
}

func (hls *highLatencyStorage) GetBlock(id blob.BlockID) ([]byte, error) {
	time.Sleep(hls.readDelay)
	return hls.Storage.GetBlock(id)
}

func uploadAndTime(omgr cas.ObjectManager, dir string, previous cas.ObjectID) *fs.UploadResult {
	log.Println("---")
	uploader, err := fs.NewUploader(omgr)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	omgr.ResetStats()
	t0 := time.Now()
	res, err := uploader.UploadDir(dir, previous)
	if err != nil {
		log.Fatalf("Error uploading: %v", err)
	}
	dt := time.Since(t0)

	log.Printf("Uploaded: %v in %v", res.ObjectID, dt)
	log.Printf("Stats: %#v", omgr.Stats())
	return res
}

type subdirEntry struct {
	entry      *fs.Entry
	dirChannel chan fs.Directory
}

var parallelReads int32

type gantt struct {
	dir  string
	from time.Time
	to   time.Time
}

var allGantt []*gantt
var ganttMutex sync.Mutex

func walkTree2(ch chan *fs.Entry, omgr cas.ObjectManager, path string, dir fs.Directory) {
	//log.Printf("walkTree2(%s)", path)
	m := map[int]chan fs.Directory{}

	// Channel containing channels with subdirectory contents, one for each subdirectory in 'dir'.
	subdirChannels := make(chan *subdirEntry, 10)

	subdirCount := 0
	for i, e := range dir {
		if e.FileMode.IsDir() {
			m[i] = make(chan fs.Directory, 10)
			subdirCount++
		}
	}

	workerCount := subdirCount
	if workerCount > maxWorkerCount {
		workerCount = maxWorkerCount
	}
	for i := 0; i < workerCount; i++ {
		go func(id int) {
			for {
				//log.Printf("worker %v waiting for work", id)
				se, ok := <-subdirChannels
				if !ok {
					//log.Printf("worker %v quitting", id)
					break
				}
				defer close(se.dirChannel)

				//log.Printf("worker %v loading %v", id, se.entry.ObjectID())
				//defer close(se.dirChannel)

				d, err := omgr.Open(se.entry.ObjectID)
				if err != nil {
					log.Printf("ERROR: %v", err)
					return
				}

				//log.Printf("loading directory %v with prefix %v", se.entry.Name(), path +"/" + se.)

				g := &gantt{}
				g.from = time.Now()

				atomic.AddInt32(&parallelReads, 1)
				dir, err := fs.ReadDirectory(d, se.entry.Name+"/")
				atomic.AddInt32(&parallelReads, -1)
				g.to = time.Now()
				g.dir = se.entry.Name

				ganttMutex.Lock()
				allGantt = append(allGantt, g)
				ganttMutex.Unlock()

				if err != nil {
					log.Printf("ERROR: %v", err)
					return
				}

				se.dirChannel <- dir
			}
		}(i)
	}

	go func() {
		for i, e := range dir {
			if m[i] != nil {
				subdirChannels <- &subdirEntry{
					entry:      e,
					dirChannel: m[i],
				}
			}
		}
		close(subdirChannels)
	}()

	for i, e := range dir {
		//log.Printf("%v[%v] = %v", path, i, e.Name())
		if e.FileMode.IsDir() {
			subdir := <-m[i]
			walkTree2(ch, omgr, e.Name+"/", subdir)
		}
		ch <- e
	}
}

func walkTree(omgr cas.ObjectManager, path string, oid cas.ObjectID) chan *fs.Entry {
	ch := make(chan *fs.Entry, 20)
	go func() {
		d, err := omgr.Open(oid)
		defer close(ch)
		if err != nil {
			log.Printf("ERROR: %v", err)
			return
		}

		dir, err := fs.ReadDirectory(d, path)
		if err != nil {
			log.Printf("ERROR: %v", err)
			return
		}

		walkTree2(ch, omgr, path, dir)
	}()
	return ch
}

func readCached(omgr cas.ObjectManager, manifestOID cas.ObjectID) {
	var r io.Reader
	var err error
	r, err = omgr.Open(manifestOID)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	//r, err = gzip.NewReader(r)

	t0 := time.Now()
	//fs.ReadDirectory(r, "")
	v, _ := ioutil.ReadAll(r)
	log.Printf("%v", string(v))
	dt := time.Since(t0)
	log.Printf("parsed in %v", dt)
}

func main() {
	var e fs.Entry

	log.Println(unsafe.Sizeof(e))

	data := map[string][]byte{}
	st := blob.NewMapStorage(data)

	// st = &highLatencyStorage{
	// 	Storage:    st,
	// 	writeDelay: 0 * time.Millisecond,
	// 	readDelay:  0 * time.Millisecond,
	// }
	format := cas.Format{
		Version: "1",
		Hash:    "md5",
	}

	omgr, err := cas.NewObjectManager(st, format)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	time.Sleep(1 * time.Second)

	path := "/Users/jarek/Projects/Kopia/src/github.com/kopia/"

	r1 := uploadAndTime(omgr, path, "")
	log.Printf("finished: %#v", *r1)
	r2 := uploadAndTime(omgr, path, r1.ManifestID)
	log.Printf("finished second time: %#v", *r2)
	//readCached(omgr, r2.ManifestID)
}