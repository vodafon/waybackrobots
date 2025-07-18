package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	flagDomain    = flag.String("d", "", "target domain")
	flagStartDate = flag.String("start_date", "", "Start date in format 'YYYY-MM-DD'. Ex. '2025-01-24'. If empty equal to '1970-01-01'")
	flagEndDate   = flag.String("end_date", "", "End date in format 'YYYY-MM-DD'. Ex. '2025-01-24'. If empty equal to current date")
	flagProcs     = flag.Int("c", 1, "concurrency")
	flagRaw       = flag.Bool("raw", false, "raw lines, not only disallowed")
	flagTimeout   = flag.Duration("timeout", 30*time.Second, "timeout for HTTP requests")

	listFormat     = "https://web.archive.org/cdx/search/cdx?url=%s/robots.txt&output=json&fl=timestamp,original&filter=statuscode:200&collapse=digest"
	snapshotFormat = "https://web.archive.org/web/%sif_/%s"
)

type Uniq struct {
	sync.Mutex
	mp map[string]struct{}
}

func (obj *Worker) printUniq(el string) {
	obj.um.Lock()
	defer obj.um.Unlock()

	if _, ok := obj.um.mp[el]; ok {
		return
	}

	obj.um.mp[el] = struct{}{}

	fmt.Fprintln(obj.w, el)
}

type Worker struct {
	wg       *sync.WaitGroup
	rowC     chan [2]string
	um       *Uniq
	cl       client
	w        io.Writer
	rawLines bool
}

func (w Worker) Do() {
	for row := range w.rowC {
		w.processRow(row)
		w.wg.Done()
	}
}

type client struct {
	httpClient *http.Client
	sdate      time.Time
	edate      time.Time
}

func main() {
	flag.Parse()

	if *flagDomain == "" || *flagProcs < 1 {
		flag.PrintDefaults()
		os.Exit(1)
	}
	httpclient := &http.Client{
		Timeout: *flagTimeout, // set 10s timeout
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Returning this error stops following redirects
			return http.ErrUseLastResponse
		},
	}
	cl := client{
		httpClient: httpclient,
		sdate:      mustParseStartDate(*flagStartDate),
		edate:      mustParseEndDate(*flagEndDate),
	}

	list := listSnapshots(cl)
	if len(list) == 0 {
		log.Println("Not Found")
		return
	}

	log.Printf("Found %d files\n", len(list))

	processSnapshots(list, cl, *flagRaw)
}

func mustParseStartDate(sd string) time.Time {
	if sd == "" {
		sd = "1970-01-01"
	}
	startDate, err := time.Parse("2006-01-02", sd)
	if err != nil {
		log.Fatalf("Invalid '-start_date' argument")
	}
	return startDate
}

func mustParseEndDate(sd string) time.Time {
	if sd == "" {
		return time.Now()
	}
	endDate, err := time.Parse("2006-01-02", sd)
	if err != nil {
		log.Fatalf("Invalid '-end_date' argument")
	}
	return endDate
}

func (w Worker) processRow(row [2]string) {
	u := fmt.Sprintf(snapshotFormat, row[0], row[1])

	resp, err := w.cl.httpClient.Get(u)
	if err != nil {
		log.Printf("[Process] WARN: fetch snapshot for %s error: %s", row, err)
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 1024*4) // 4Mb
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			log.Printf("[Process] WARN: read snapshot for %s error: %s", row, err)
			return
		}
		log.Fatalf("[Process] Read body from %q error: %v", u, err)
	}
	data := buf[:n]
	if isInvalidResponse(strings.TrimSpace(strings.ToLower(string(data)))) {
		return
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := scanner.Text()
		if w.rawLines {
			fmt.Fprintln(w.w, line)
			continue
		}

		if !strings.HasPrefix(strings.ToLower(line), "disallow:") {
			continue
		}

		pat := strings.TrimSpace(line[9:])
		if len(pat) == 0 {
			continue
		}

		w.printUniq(pat)
	}
}

func isInvalidResponse(dataLow string) bool {
	// HTML
	if strings.Contains(dataLow, "<!doctype ") || strings.Contains(dataLow, "<html") || strings.Contains(dataLow, "<body") {
		return true
	}
	// JSON
	if (strings.HasPrefix(dataLow, "{") && strings.HasSuffix(dataLow, "}")) || (strings.HasPrefix(dataLow, "[") && strings.HasSuffix(dataLow, "]")) {
		return true
	}
	return false
}

func processSnapshots(list [][2]string, cl client, rawLines bool) {
	wg := &sync.WaitGroup{}
	rowC := make(chan [2]string, *flagProcs)
	uniq := &Uniq{
		mp: make(map[string]struct{}),
	}

	for i := 0; i < *flagProcs; i++ {
		go Worker{
			wg:       wg,
			rowC:     rowC,
			um:       uniq,
			cl:       cl,
			w:        os.Stdout,
			rawLines: rawLines,
		}.Do()
	}

	wg.Add(len(list))

	for _, row := range list {
		rowC <- row
	}

	close(rowC)
	wg.Wait()
}

func listSnapshots(cl client) [][2]string {
	u := fmt.Sprintf(listFormat, *flagDomain)

	resp, err := cl.httpClient.Get(u)
	if err != nil {
		log.Fatalf("[List] GET %q error: %v", u, err)
	}

	defer resp.Body.Close()

	res := [][2]string{}
	tmp := [][2]string{}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("[List] Read body from %q error: %v", u, err)
	}

	if resp.StatusCode == 403 && bytes.Contains(data, []byte("AdministrativeAccessControlException: Blocked Site Error")) {
		log.Fatal("This domain has been manually excluded from the Wayback Machine")
	}

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		log.Fatalf("[List] %q: JSON decode for %q error: %v", u, data, err)
	}

	if len(tmp) < 2 {
		return [][2]string{}
	}

	// without header row
	for _, el := range tmp[1:] {
		ts := el[0]
		t, err := time.Parse("20060102150405", ts) // Wayback format
		if err != nil {
			continue
		}
		if t.Before(cl.sdate) || t.After(cl.edate) {
			continue
		}
		res = append(res, el)
	}

	// Sort descending
	sort.Slice(res, func(i, j int) bool {
		return res[i][0] > res[j][0]
	})

	return res
}
