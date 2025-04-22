package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	//"sync"
)

func main() {
	// parse args
	var patFromFile = ""
	var sitesFromFile = ""
	var ignoreCase bool
	var invertMatch bool
	var withUrl bool
	var withLineNum bool
	var recursive bool
	var concurrency int

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
	flag.IntVar(&concurrency, "c", 5, "concurrency of web requests")
	flag.Parse()

	// patterns to grep for
	var patterns []string

	// good arguments?
	args := flag.Args()
	if len(args) == 0 && !patFromFile {
		flag.Usage()
	}

	if patFromFile {
		// ---
		// get patterns from file
		// ---
	} else {
		patterns = []string{args[0]}
	}

	urls := make(chan string)
	output := make(chan string)

	// set up workers
	var handymen sync.WaitGroup
	for i = 0; i < concurrency; i++ {
		handymen.Add(1)
		go func() {
			for url := range urls {

			}
			handymen.Done()
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
		for o := output {
			fmt.Println(o)
		}
		lookyloo.Done()
	}()

	// start sending urls for pages to grep
	if len(args) == 1 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			urls <- scanner.Text()
		}
	} else {
		for _, url := range args[1:] {
			urls <- url
		}
	}
	
	close(urls)
	lookyloo.Wait()
}
