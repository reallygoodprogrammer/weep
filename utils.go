package main

import (
	"bufio"
	"fmt"
	"golang.org/x/term"
	"os"
	"strings"
)

// color stuff
const MatchColor = "\033[1;32m"
const BadColor = "\033[1;31m"
const ResetColor = "\033[0m"

// Failure
//
// format a failure message to be sent to 'output' channel
func failure[T any](err T, url string) {
	fmt.Fprintln(os.Stderr, badlight(fmt.Sprintf("failure: %s, url: '%s'", err, url), "failure"))
}

// Highlight
//
// highlight's `match` in `input` using highlight color
func highlight(input string, match string) string {
	return lightUp(input, match, MatchColor)
}

// Badlight
//
// highlight's `match` in `input` using bad color
func badlight(input string, match string) string {
	return lightUp(input, match, BadColor)
}

// Light Up
//
// lights up matching content in the line, ignoring case
func lightUp(input string, match string, color string) string {
	ind := strings.Index(strings.ToLower(input), strings.ToLower(match))
	if ind < 0 {
		panic("no matching value within input")
	}
	fmt.Println(input, match)
	return string(input[0:ind]) + color + string(input[ind:ind+len(match)]) +
		ResetColor + string(input[ind+len(match):])
}

// Load From File
//
// load lines from a file, panic if failure
func loadFromFile(filename string) []string {
	lines := []string{}
	if filename != "" {
		file, err := os.Open(filename)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
	}
	return lines
}

// Check if `out` is a tty
func isTTY(out *os.File) bool {
	return term.IsTerminal(int(out.Fd()))
}
