# weep (web-e grep)

A program for recursively greping webpage responses with my most frequent
grep options available.

---

## usage

```
Usage: weep [-cEHinsv] [-d value] [-f value] [-l value] [-o value] [-t value] [parameters ...]
 -c        match text within tag by a css selector
 -d value  obtain allowed domains to search from file argument
 -E        treat patterns as regular expressions (RE2)
 -f value  obtain patterns from file argument
 -H        display URL of matching page before line
 -i        ignore case of input/patterns
 -l value  rate of requests per second (default: none)
 -n        display line number of matching line
 -o value  output file name to write matches too
 -s        do not recursively search for new pages (single request)
 -t value  concurrency of web requests (default 10) [10]
 -v        only return non-martching lines
```

---

## licensing

License an be found [here](LICENSE.txt).
