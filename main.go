package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/pborman/getopt/v2"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
)

// settings struct
var settings WeepSettings

// channels / waitgroup
var urls chan string
var output chan string
var urlCount sync.WaitGroup
var urlList []string
var urlListMut sync.RWMutex

// client
var client http.Client = http.Client{}

func main() {
	var concurrency = 10
	var rateLimit = 0.0
	var args []string

	settings = NewWeepSettings()

	// parse arguments
	func() {
		var patternsFile = ""
		var allowedDomainsFile = ""
		var allowedDomains = ""
		var outputFile = ""

		getopt.Flag(&patternsFile, 'F', "obtain matching patterns from file argument")
		getopt.Flag(&allowedDomainsFile, 'D', "obtain allowed domains to search from file argument")
		getopt.Flag(&allowedDomains, 'd', "allowed domains string with each hostname separated by ','")
		getopt.Flag(&settings.OutputFile, 'o', "output file name to write matching content too")
		getopt.Flag(&settings.IgnoreCase, 'i', "ignore case of input/patterns")
		getopt.Flag(&settings.InvertMatch, 'v', "only return non-matching lines")
		getopt.Flag(&settings.WithLineNum, 'n', "prefix line number onto matching line")
		getopt.Flag(&settings.WithUrl, 'H', "display URL of matching page before line")
		getopt.Flag(&settings.Single, 's', "do not recursively search for new pages (single request)")
		getopt.Flag(&settings.RegexPatterns, 'E', "treat patterns as regular expressions (RE2)")
		getopt.Flag(&settings.CSSPatterns, 'c', "find text within tag by a matching css selector")
		getopt.Flag(&concurrency, 't', "concurrency of web requests (default: 10)")
		getopt.Flag(&rateLimit, 'r', "rate of requests per second (default: none)")
		getopt.Parse()
		args = getopt.Args()

		if len(args) == 0 && patternsFile == "" {
			getopt.Usage()
			os.Exit(1)
		}

		settings.SetOutputFile(outputFile)
		settings.SetRateLimit(rateLimit)
		if patternsFile != "" {
			settings.SetPatternFile(patternsFile)
		} else {
			settings.SetPattern(args[0])
		}
		settings.SetAllowedDomainsFile(allowedDomainsFile)
		settings.SetAllowedDomains(allowedDomains)
	}()

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
				} else {
					urlCount.Done()
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
			fmt.Fprintln(settings.Out, o)
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

// Deal With Requests
//
// Function for requesting routines to find matches, get new urls
func dealWithReq(u string) {
	defer urlCount.Done()

	// handles panics within go routine
	defer func() {
		if r := recover(); r != nil {
			panic(r)
			//failure(r, u)
		}
	}()

	// create request
	req := must(http.NewRequest(settings.RequestMethod, u, nil))

	// limit request if necessary
	if err := settings.Limiter.Wait(context.Background()); err != nil {
		panic(err)
	}

	// make request, get body
	resp := must(client.Do(req))
	body := must(io.ReadAll(resp.Body))
	resp.Body.Close()

	// find matches
	var finderWG sync.WaitGroup
	finderWG.Add(1)
	defer finderWG.Wait()
	go func() {
		defer finderWG.Done()
		results := settings.FindMatches(&body, u)
		for _, r := range results {
			output <- r
		}
	}()

	// go to other links
	if !settings.Single {
		lus := extractLinks(&body, u)

		// either send url to channel, or deal with it yourself
		for _, lu := range lus {
			urlCount.Add(1)
			select {
			case urls <- lu:
				continue
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
func extractLinks(body *[]byte, u string) []string {
	dirPrefix := must(url.Parse(u))
	dirPrefix.Path += "/"

	htmlDoc := must(goquery.NewDocumentFromReader(bytes.NewReader(*body)))

	links := []string{}
	htmlDoc.Find("[href], [src]").Each(func(i int, selection *goquery.Selection) {
		link, found := selection.Attr("src")
		if !found {
			link, _ = selection.Attr("href")
		}

		newUrl, err := url.Parse(link)
		if err != nil {
			newUrl = dirPrefix.ResolveReference(newUrl)
		}

		hostname := newUrl.Hostname()
		if hostname == dirPrefix.Hostname() {
			links = append(links, newUrl.String())
		} else {
			for _, d := range settings.AllowedDomains {
				if hostname == d {
					links = append(links, newUrl.String())
					break
				}
			}
		}
	})
	return links
}

// function for handling error values
func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
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
	if u[len(u)-1] == '/' {
		u = string(u[:len(u)-1])
	}

	doIt := true
	urlListMut.RLock()
	for _, visited := range urlList {
		if u == visited {
			doIt = false
			break
		}
	}
	urlListMut.RUnlock()
	if doIt {
		urlListMut.Lock()
		urlList = append(urlList, u)
		urlListMut.Unlock()
	}
	return doIt
}
