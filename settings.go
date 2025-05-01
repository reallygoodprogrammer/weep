package main

import (
	"golang.org/x/time/rate"
	"os"
	"regexp"
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
	// Regular expressions
	regex []*regexp.Regexp
	// Treat patterns as css selectors
	CSSPatterns bool

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

// Check if `line` matches matching criteria
func (settings *WeepSettings) IsMatch(line string, lineNum int, u string) (string, bool) {
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
				matches := reg.FindAllString(inputLine, -1)
				for _, m := range matches {
					markedLine = highlight(markedLine, m)
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
				markedLine = highlight(markedLine, pat)
			} else if settings.InvertMatch {
				match = true
				break
			}
		}
	}

	return markedLine, match
}
