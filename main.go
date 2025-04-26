package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"golang.org/x/net/html"
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

// channels / waitgroup
var urls chan string
var output chan string
var urlCount sync.WaitGroup

// client and limiter
var client http.Client = http.Client{}
var limiter *rate.Limiter

func main() {
	// parse args
	var patFromFile = ""
	var sitesFromFile = ""
	var concurrency int
	var rateLimit float64

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: cat <urls-file> | ./wepp <pattern> [url+]\n")
		flag.PrintDefaults()
	}

	flag.StringVar(&patFromFile, "f", "", "obtain patterns from file argument")
	flag.StringVar(&sitesFromFile, "s", "", "obtain urls from file argument")
	flag.BoolVar(&ignoreCase, "i", false, "ignore cases of input and patterns")
	flag.BoolVar(&invertMatch, "v", false, "only return non-martching lines")
	flag.BoolVar(&withLineNum, "n", false, "display line number of matching line")
	flag.BoolVar(&withUrl, "H", false, "display URL of matching page before line")
	flag.BoolVar(&recursive, "r", false, "recursively search url directory using links in page")
	flag.IntVar(&concurrency, "c", 1, "concurrency of web requests")
	flag.Float64Var(&rateLimit, "l", 0.0, "rate of requests per second")
	flag.Parse()

	// good arguments?
	args := flag.Args()
	if len(args) == 0 && patFromFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	if rateLimit > 0 {
		limiter = rate.NewLimiter(rate.Every(time.Duration(rateLimit*float64(time.Second))), 1)
	} else {
		limiter = rate.NewLimiter(rate.Inf, 100)
	}

	if patFromFile != "" {
		file, err := os.Open(patFromFile)
		if err != nil {
			panic(err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			patterns = append(patterns, scanner.Text())
		}
		file.Close()
	} else {
		patterns = []string{args[0]}
	}

	urls = make(chan string)
	output = make(chan string)

	// set up workers
	var handymen sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		handymen.Add(1)

		go func() {
			defer handymen.Done()

			for u := range urls {
				dealWithReq(u)
			}
		}()
	}

	go func() {
		handymen.Wait()
		close(output)
	}()

	// set up output
	var lookyloo sync.WaitGroup
	lookyloo.Add(1)
	go func() {
		for o := range output {
			fmt.Println(o)
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
			// format output line
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
				dealWithReq(lu)
			}
		}
	}
}

// extract the urls from 'src' and 'href' attributes
func extractLinks(n *html.Node, u string) ([]string, error) {
	urlDir, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	links := []string{}
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "href" || attr.Key == "src" {
				newu, err := url.Parse(attr.Val)
				if err == nil {
					newu = urlDir.ResolveReference(newu)
				}
				links = append(links, newu.String())
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

// get the directory prefix of `fullUrl`
func removeFile(fullUrl string) (string, error) {
	u, err := url.Parse(fullUrl)
	if err != nil {
		return fullUrl, err
	}
	u.Path = path.Dir(u.Path) + "/"
	return u.String(), nil
}

// format a failure message to be sent to 'output' channel
func failure(err error, url string) {
	output <- badlight(fmt.Sprintf("failure: %s, url: '%s'", err, url), "failure")
}

// highlight's `match` in `input` using highlight color
func highlight(input string, match string) string {
	return lightUp(input, match, MatchColor)
}

// highlight's `match` in `input` using bad color
func badlight(input string, match string) string {
	return lightUp(input, match, BadColor)
}

func lightUp(input string, match string, color string) string {
	h := color + match + ResetColor
	return strings.ReplaceAll(input, match, h)
}
