package main

import (
	"golang.org/x/time/rate"
	"os"
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
	// only run a single url (do not recurse through page)
	Single bool
	// request method to use
	RequestMethod string
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
	// allowed domains to recurse through
	AllowedDomains []string
	// Limiter for rate limiting requests
	Limiter *rate.Limiter
}

func NewWeepSettings() WeepSettings {
	return WeepSettings{
		RequestMethod: "GET",
		Out: os.Stdout,
		OutputFile: "",
		IsTTY: isTTY(os.Stdout),
		ErrIsTTY: isTTY(os.Stderr),
		Patterns: []string{},
		AllowedDomains: []string{},
		Limiter: rate.NewLimiter(rate.Inf, 100),
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
	settings.Patterns = append(settings.Patterns, loadFromFile(patternFile)...)
}

func (settings *WeepSettings) SetPattern(pattern string) {
	settings.Patterns = append(settings.Patterns, pattern)
}

func (settings *WeepSettings) SetAllowedDomainsFile(allowedDomainsFile string) {
	settings.AllowedDomains = append(settings.AllowedDomains, loadFromFile(allowedDomainsFile)...)
}

// Check if `line` matches matching criteria
func (settings *WeepSettings) IsMatch(line string, lineNum int, u string) (string, bool) {
	match := false
	markedLine := line
	if settings.IgnoreCase {
		lowerLine := strings.ToLower(line)
		for _, pat := range settings.Patterns {
			lowerPattern := strings.ToLower(pat)
			if strings.Contains(lowerLine, lowerPattern) && !settings.InvertMatch {
				match = true
				markedLine = strings.TrimSpace(highlight(markedLine, pat))
			} else if settings.InvertMatch {
				match = true
				break
			}
		}
	} else {
		for _, pat := range settings.Patterns {
			if strings.Contains(line, pat) && !settings.InvertMatch {
				match = true
				markedLine = strings.TrimSpace(highlight(markedLine, pat))
			} else if settings.InvertMatch {
				match = true
				break
			}
		}
	}
	return markedLine, match
}
