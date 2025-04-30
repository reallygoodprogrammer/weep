package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"golang.org/x/term"
	"golang.org/x/time/rate"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// color stuff
const MatchColor = "\033[1;32m"
const BadColor = "\033[1;31m"
const ResetColor = "\033[0m"

// program options
var ignoreCase bool
var invertMatch bool
var withUrl bool
var withLineNum bool
var recursive bool
var reqMethod string = "GET"

// patterns to look for
var patterns []string
var allowedDomains []string

// channels / waitgroup
var urls chan string
var output chan string
var urlCount sync.WaitGroup
var urlList []string
var urlListMut sync.Mutex

// client and limiter
var client http.Client = http.Client{}
var limiter *rate.Limiter

// output
var outfd *os.File
var outputFile string
var isTTY bool
var errIsTTY bool

func main() {
	// parse args
	var patFromFile = ""
	var domainsFile = ""
	var concurrency int
	var rateLimit float64

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ./weep <pattern> [url(s) or will read stdin]\n")
		fmt.Fprintf(os.Stderr, "-> ctrl-c to stop recursive greping\n")
		flag.PrintDefaults()
	}

	flag.StringVar(&patFromFile, "f", "", "obtain patterns from file argument")
	flag.StringVar(&domainsFile, "d", "", "obtain allowed domains to search from file argument")
	flag.StringVar(&outputFile, "o", "", "output file name to write matches too")
	flag.BoolVar(&ignoreCase, "i", false, "ignore cases of input and patterns")
	flag.BoolVar(&invertMatch, "v", false, "only return non-martching lines")
	flag.BoolVar(&withLineNum, "n", false, "display line number of matching line")
	flag.BoolVar(&withUrl, "H", false, "display URL of matching page before line")
	flag.BoolVar(&recursive, "s", false, "do not recursively search for new pages (single request)")
	flag.IntVar(&concurrency, "c", 10, "concurrency of web requests (default 10)")
	flag.Float64Var(&rateLimit, "l", 0.5, "rate of requests per second (default: none)")
	flag.Parse()

	// good arguments?
	args := flag.Args()
	if len(args) == 0 && patFromFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	recursive = !recursive

	// set up output file
	if outputFile != "" {
		var err error
		outfd, err = os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			panic(err)
		}
	} else {
		outfd = os.Stdout
	}
	isTTY = term.IsTerminal(int(outfd.Fd()))
	errIsTTY = term.IsTerminal(int(os.Stderr.Fd()))

	// set up rate limiter
	if rateLimit > 0 {
		limiter = rate.NewLimiter(rate.Every(time.Duration(rateLimit*float64(time.Second))), 1)
	} else {
		limiter = rate.NewLimiter(rate.Inf, 100)
	}

	// load file options
	if patFromFile != "" {
		patterns = loadFromFile(patFromFile)
	} else {
		patterns = []string{args[0]}
	}

	if domainsFile != "" {
		allowedDomains = loadFromFile(domainsFile)
	} else {
		allowedDomains = []string{}
	}

	// set up workers
	urls = make(chan string)
	output = make(chan string)

	var handymen sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		handymen.Add(1)

		go func() {
			defer handymen.Done()

			for u := range urls {
				if checkAndAppendUrl(u) {
					dealWithReq(u)
				}
			}
		}()
	}

	go func() {
		handymen.Wait()
		close(output)
	}()

	var lookyloo sync.WaitGroup
	lookyloo.Add(1)
	go func() {
		for o := range output {
			fmt.Fprintln(outfd, o)
		}
		lookyloo.Done()
	}()

	// start sending urls for pages to grep
	if len(args) == 1 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			urlCount.Add(1)
			urls <- scanner.Text()
		}
	} else {
		for _, url := range args[1:] {
			urlCount.Add(1)
			urls <- url
		}
	}

	// wait for all urls to be worked on
	urlCount.Wait()
	close(urls)
	// wait for output
	lookyloo.Wait()
}

// deal with making request, extracting matches and urls from response
// sends formatted matches to 'output' channel, urls to 'urls' channel
// also sends failure messages to 'output' channel
func dealWithReq(u string) {
	defer urlCount.Done()

	// make request
	req, err := http.NewRequest(reqMethod, u, nil)
	if err != nil {
		failure(err, u)
		return
	}

	if err := limiter.Wait(context.Background()); err != nil {
		failure(err, u)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		failure(err, u)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		failure(err, u)
		return
	}
	resp.Body.Close()

	// scan over response
	scanner := bufio.NewScanner(bytes.NewReader(body))
	lineNum := 1
	for scanner.Scan() {
		line := scanner.Text()

		// check for matches, highlight if applicable
		match := false
		markedLine := line
		for _, pat := range patterns {
			if ignoreCase {
				if strings.Contains(strings.ToLower(line), strings.ToLower(pat)) && !invertMatch {
					match = true
					markedLine = strings.TrimSpace(highlight(strings.ToLower(markedLine), strings.ToLower(pat)))
				} else if invertMatch {
					break
				}
			} else {
				if strings.Contains(line, pat) && !invertMatch {
					match = true
					markedLine = strings.TrimSpace(highlight(markedLine, pat))
				} else if invertMatch {
					break
				}
			}
		}
		if match {
			if withLineNum {
				lineNumStr := fmt.Sprintf("%d", lineNum)
				markedLine = fmt.Sprintf("%s: %s", highlight(lineNumStr, lineNumStr), markedLine)
			}
			if withUrl {
				markedLine = fmt.Sprintf("%s: %s", highlight(u, u), markedLine)
			}
			output <- markedLine
		}
		lineNum++
	}

	if recursive {
		htmlDoc, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			failure(err, u)
			return
		}

		lus, err := extractLinks(htmlDoc, u)
		if err != nil {
			failure(err, u)
			return
		}

		// either send url to channel, or deal with it yourself
		for _, lu := range lus {
			urlCount.Add(1)
			select {
			case urls <- lu:
				urls <- lu
			default:
				if checkAndAppendUrl(lu) {
					dealWithReq(lu)
				}
			}
		}
	}
}

// Extract Links
//
// extract the urls from 'src' and 'href' attributes
func extractLinks(n *html.Node, u string) ([]string, error) {
	urlDir, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	baseDomain := urlDir.Hostname()

	links := []string{}
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "href" || attr.Key == "src" {
				newu, err := url.Parse(attr.Val)
				if err == nil {
					newu = urlDir.ResolveReference(newu)
				}
				hostname := newu.Hostname()
				if hostname == baseDomain {
					links = append(links, newu.String())
				} else {
					for _, d := range allowedDomains {
						if hostname == d {
							links = append(links, newu.String())
							break
						}
					}
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		newLinks, err := extractLinks(c, u)
		if err != nil {
			return nil, err
		}
		links = append(links, newLinks...)
	}
	return links, nil
}

// Remove File
//
// get the directory prefix of `fullUrl`
func removeFile(fullUrl string) (string, error) {
	u, err := url.Parse(fullUrl)
	if err != nil {
		return fullUrl, err
	}
	u.Path = path.Dir(u.Path) + "/"
	return u.String(), nil
}

// Failure
//
// format a failure message to be sent to 'output' channel
func failure(err error, url string) {
	fmt.Fprintln(os.Stderr, badlight(fmt.Sprintf("failure: %s, url: '%s'", err, url), "failure"))
}

// Highlight
//
// highlight's `match` in `input` using highlight color
func highlight(input string, match string) string {
	if isTTY {
		return lightUp(input, match, MatchColor)
	}
	return input
}

// Badlight
//
// highlight's `match` in `input` using bad color
func badlight(input string, match string) string {
	if errIsTTY {
		return lightUp(input, match, BadColor)
	}
	return input
}

// Light Up
func lightUp(input string, match string, color string) string {
	h := color + match + ResetColor
	return strings.ReplaceAll(input, match, h)
}

// Load From File
//
// load lines from a file
func loadFromFile(filename string) []string {
	lines := []string{}
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(patterns, scanner.Text())
	}
	return lines
}

// Check and Append Url
//
// check if a url has been worked on, if not add to urlList
func checkAndAppendUrl(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	u = parsed.String()

	doIt := true
	urlListMut.Lock()
	for _, visited := range urlList {
		if u == visited {
			doIt = false
			break
		}
	}
	if doIt {
		urlList = append(urlList, u)
	}
	urlListMut.Unlock()
	return doIt
}
