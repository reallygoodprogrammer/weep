package main

import (
//	"fmt"
	"bufio"
	"bytes"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/time/rate"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type WeepSettings struct {
	// ignore case when matching
	IgnoreCase bool
	// invert matching (find non-matching lines)
	InvertMatch bool
	// prefix url on left side of output
	WithUrl bool
	// prefix line number on left side of output
	WithLineNum bool

	// output file descriptor
	Out *os.File
	// name of output file
	OutputFile string
	// is the output file descriptor connected to a tty?
	IsTTY bool
	// is stderr connected to a tty?
	ErrIsTTY bool

	// Patterns to look for
	Patterns []string
	// Treat patterns as extended regex
	RegexPatterns bool
	// Match by attribute content rather than line content
	MatchByAttributes bool
	// Attributes to search through
	patternatts []string
	// filter for finding appropriate attributes
	patternattfilter string
	// Regular expressions
	regex []*regexp.Regexp
	// match inner html content of css selector filter
	CSSPatterns bool
	// css pattern to filter content by
	csspattern string

	// allowed domains to recurse through
	AllowedDomains []string
	// only run a single url (do not recurse through page)
	Single bool

	// request method to use
	RequestMethod string
	// Limiter for rate limiting requests
	Limiter *rate.Limiter
}

func NewWeepSettings() WeepSettings {
	return WeepSettings{
		RequestMethod:  "GET",
		Out:            os.Stdout,
		OutputFile:     "",
		IsTTY:          isTTY(os.Stdout),
		ErrIsTTY:       isTTY(os.Stderr),
		Patterns:       []string{},
		patternatts:	[]string{},
		regex:          []*regexp.Regexp{},
		AllowedDomains: []string{},
		Limiter:        rate.NewLimiter(rate.Inf, 100),
	}
}

func (settings *WeepSettings) SetOutputFile(OutputFile string) {
	if settings.OutputFile != "" {
		var err error
		settings.Out, err = os.OpenFile(settings.OutputFile, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			panic(err)
		}
		settings.IsTTY = isTTY(settings.Out)
	}
}

func (settings *WeepSettings) SetRateLimit(rateLimit float64) {
	if rateLimit > 0 {
		settings.Limiter = rate.NewLimiter(rate.Every(time.Duration(rateLimit*float64(time.Second))), 1)
	}
}

func (settings *WeepSettings) SetPatternFile(patternFile string) {
	newPatterns := loadFromFile(patternFile)
	settings.Patterns = append(settings.Patterns, newPatterns...)
	if settings.RegexPatterns {
		for _, pattern := range newPatterns {
			if settings.IgnoreCase {
				settings.regex = append(settings.regex, regexp.MustCompile(strings.ToLower(pattern)))
			} else {
				settings.regex = append(settings.regex, regexp.MustCompile(pattern))
			}
		}
	}
}

func (settings *WeepSettings) SetPattern(pattern string) {
	settings.Patterns = append(settings.Patterns, pattern)
	if settings.RegexPatterns {
		if settings.IgnoreCase {
			settings.regex = append(settings.regex, regexp.MustCompile(strings.ToLower(pattern)))
		} else {
			settings.regex = append(settings.regex, regexp.MustCompile(pattern))
		}
	}
}

func (settings *WeepSettings) SetAllowedDomainsFile(allowedDomainsFile string) {
	settings.AllowedDomains = append(settings.AllowedDomains, loadFromFile(allowedDomainsFile)...)
}

func (settings *WeepSettings) SetAllowedDomains(allowedDomains string) {
	if allowedDomains != "" {
		settings.AllowedDomains = append(settings.AllowedDomains, strings.Split(allowedDomains, ",")...)
	}
}

func (settings *WeepSettings) SetMatchAttributes(patternAttributes string) {
	if patternAttributes != "" {
		settings.MatchByAttributes = true
		settings.patternatts = append(settings.patternatts, strings.Split(patternAttributes, ",")...)
		bracket := []string{}
		for _, p := range settings.patternatts {
			bracket = append(bracket, "["+p+"]")
		}
		settings.patternattfilter = strings.Join(bracket, " , ")
	}
}

func (settings *WeepSettings) SetMatchCSS(patternCSS string) {
	if patternCSS != "" {
		settings.CSSPatterns = true
		settings.csspattern = patternCSS
	}
}

func (settings *WeepSettings) FindMatches(body *[]byte, u string) []string {
	results := []string{}

	checkAndAdd := func(line string, lineNum int) bool {
		markedLine, match := settings.IsMatch(line)
		if match {
			results = append(results, settings.formatted(markedLine, lineNum, u))
			return true
		}
		return false
	}

	matchByAttr := func(doc *goquery.Selection) {
		doc.Find(settings.patternattfilter).Each(func(i int, result *goquery.Selection) {
			for _, a := range settings.patternatts {
				attr, found := result.Attr(a)
				if !found {
					continue
				}
				if checkAndAdd(attr, -1) {
					break
				}
			}
		})
	}

	if settings.CSSPatterns {
		htmlDoc := must(goquery.NewDocumentFromReader(bytes.NewReader(*body)))
		for _, p := range settings.Patterns {
			htmlDoc.Find(p).Each(func(i int, result *goquery.Selection) {
				if settings.MatchByAttributes {
					matchByAttr(result)
				} else {
					value, err := result.Html()
					if err == nil {
						checkAndAdd(value, -1)
					}
				}
			})
		}
	} else if settings.MatchByAttributes {
		htmlDoc := must(goquery.NewDocumentFromReader(bytes.NewReader(*body)))
		matchByAttr(htmlDoc.Selection)
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(*body))
		lineNum := 1
		for scanner.Scan() {
			checkAndAdd(scanner.Text(), lineNum)
			lineNum++
		}
	}

	return results
}

// Check if `line` matches matching criteria
func (settings *WeepSettings) IsMatch(line string) (string, bool) {
	match := false
	markedLine := strings.TrimSpace(line)
	inputLine := line
	if settings.IgnoreCase {
		inputLine = strings.ToLower(inputLine)
	}

	if settings.RegexPatterns {
		for _, reg := range settings.regex {
			if reg.MatchString(inputLine) && !settings.InvertMatch {
				match = true
				if settings.IsTTY {
					matches := reg.FindAllString(inputLine, -1)
					for _, m := range matches {
						markedLine = highlight(markedLine, m)
					}
				}
			}
		}
	} else {
		for _, pat := range settings.Patterns {
			inputPattern := pat
			if settings.IgnoreCase {
				inputPattern = strings.ToLower(pat)
			}
			if strings.Contains(inputLine, inputPattern) && !settings.InvertMatch {
				match = true
				if settings.IsTTY {
					markedLine = highlight(markedLine, pat)
				}
			} else if settings.InvertMatch {
				match = true
				break
			}
		}
	}

	return markedLine, match
}

func (settings *WeepSettings) formatted(line string, lineNumVal int, url string) string {
	lineNum := strconv.Itoa(lineNumVal)
	if settings.IsTTY {
		lineNum = highlight(lineNum, lineNum)
		url = highlight(url, url)
	}
	if lineNumVal > 0 && settings.WithLineNum {
		line = string(lineNum) + ": " + line
	}
	if settings.WithUrl {
		line = url + ": " + line
	}
	return line
}
