package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	flagDomain = flag.String("d", "", "target domain")
	flagProcs  = flag.Int("c", 10, "concurrency")

	listFormat     = "https://web.archive.org/cdx/search/cdx?url=%s/robots.txt&output=json&fl=timestamp,original&filter=statuscode:200&collapse=digest"
	snapshotFormat = "https://web.archive.org/web/%sif_/%s"
)

type Uniq struct {
	sync.Mutex
	mp map[string]struct{}
}

func (obj *Uniq) printUniq(el string) {
	obj.Lock()
	defer obj.Unlock()

	if _, ok := obj.mp[el]; ok {
		return
	}

	obj.mp[el] = struct{}{}

	fmt.Println(el)
}

type Worker struct {
	wg   *sync.WaitGroup
	rowC chan [2]string
	um   *Uniq
}

func (w Worker) Do() {
	for row := range w.rowC {
		w.processRow(row)
		w.wg.Done()
	}
}

func main() {
	flag.Parse()

	if *flagDomain == "" || *flagProcs < 1 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	list := listSnapshots()
	if len(list) == 0 {
		log.Println("Not Found")
		return
	}

	log.Printf("Found %d files\n", len(list))

	processSnapshots(list)
}

func (w Worker) processRow(row [2]string) {
	u := fmt.Sprintf(snapshotFormat, row[0], row[1])

	resp, err := http.Get(u)
	if err != nil {
		log.Printf("WARN: fetch snapshot for %s error: %s", row, err)
		return
	}

	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(strings.ToLower(line), "disallow:") {
			continue
		}

		pat := strings.TrimSpace(line[9:])
		if len(pat) == 0 {
			continue
		}

		w.um.printUniq(pat)
	}
}

func processSnapshots(list [][2]string) {
	wg := &sync.WaitGroup{}
	rowC := make(chan [2]string, *flagProcs)
	uniq := &Uniq{
		mp: make(map[string]struct{}),
	}

	for i := 0; i < *flagProcs; i++ {
		go Worker{
			wg:   wg,
			rowC: rowC,
			um:   uniq,
		}.Do()
	}

	wg.Add(len(list))

	for _, row := range list {
		rowC <- row
	}

	close(rowC)
	wg.Wait()
}

func listSnapshots() [][2]string {
	u := fmt.Sprintf(listFormat, *flagDomain)

	resp, err := http.Get(u)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	res := [][2]string{}

	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		log.Fatal(err)
	}

	if len(res) < 2 {
		return [][2]string{}
	}

	// without header row
	return res[1:]
}
