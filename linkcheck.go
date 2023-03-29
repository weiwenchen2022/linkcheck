package linkcheck

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/antchfx/htmlquery"
)

const (
	numPollers = 12 // number of Poller goroutines to launch
)

type State struct {
	source string
	link   string
	status int
}

type response struct {
	link string
	resp *http.Response
}

type LinkChecker struct {
	mu      sync.Mutex
	visited map[string]State

	base  *url.URL
	start string

	pending  chan string
	complete chan response
	wg       sync.WaitGroup
}

func NewLinkChecker() *LinkChecker {
	l := LinkChecker{
		visited: make(map[string]State),

		pending:  make(chan string, numPollers),
		complete: make(chan response, numPollers),
	}

	go l.parse()

	for i := 0; i < numPollers; i++ {
		go l.crawl()
	}

	return &l
}

func (l *LinkChecker) Main(link string) {
	tmp, err := url.Parse(link)
	if err != nil {
		log.Fatal(err)
	}

	l.base = tmp
	l.start = link
	l.visited[link] = State{link, link, 0}

	l.wg.Add(1)
	l.pending <- link

	l.wg.Wait()
	close(l.pending)
	close(l.complete)

	var b strings.Builder
	brokens := 0
	for k, v := range l.visited {
		if v.status != http.StatusOK {
			brokens++
			fmt.Fprintf(&b, "\n%d. Source page: %s\n", brokens, v.source)
			fmt.Fprintf(&b, "Target: %s (status %d)\n", k, v.status)
		}
	}

	log.Printf("%d links scanned, %d broken links found:\n",
		len(l.visited), brokens)
	log.Println(b.String())
}

func (l *LinkChecker) crawl() {
	for link := range l.pending {
		resp, err := http.Get(link)
		if err != nil {
			log.Println(link, err)
			l.wg.Done()
			continue
		}

		l.mu.Lock()
		if st, ok := l.visited[link]; !ok {
			panic("not found status")
		} else {
			st.status = resp.StatusCode
			l.visited[link] = st
		}
		l.mu.Unlock()

		if resp.StatusCode != http.StatusOK {
			log.Println(link, resp.Status)
			resp.Body.Close()
			l.wg.Done()
			continue
		}

		log.Println(link, resp.Status)

		if isOffsite(link, l.start) {
			log.Println(link, "offsite")
			resp.Body.Close()
			l.wg.Done()
			continue
		}

		l.complete <- response{link, resp}
	}
}

func (l *LinkChecker) parse() {
	for v := range l.complete {
		func() {
			defer l.wg.Done()
			defer v.resp.Body.Close()

			doc, err := htmlquery.Parse(v.resp.Body)
			if err != nil {
				log.Println(v.link, err)
				return
			}

			list := htmlquery.Find(doc, "//a/@href")
			for _, n := range list {
				link := htmlquery.SelectAttr(n, "href")
				u, err := url.Parse(link)
				if err != nil {
					log.Println(link, err)
					return
				}

				link = l.base.ResolveReference(u).String()

				l.mu.Lock()
				if _, ok := l.visited[link]; ok {
					log.Println(link, "skipping")
				} else {
					l.visited[link] = State{v.link, link, 0}

					go func() {
						log.Println(link, "crawling...")

						l.wg.Add(1)
						l.pending <- link
					}()
				}
				l.mu.Unlock()
			}
		}()
	}
}

func isOffsite(link, start string) bool {
	return !strings.HasPrefix(link, start)
}
