# weep (web-e grep)

A go program for greping webpages with my most used grep options.

## TODO

- find url paths that are relative in recursive mode
- find url paths based on html fields 'href, src, etc.'
- implement request headers?

## usage

```
usage: cat <urls-file> | ./wepp <pattern> [url+]
  -H	display URL of matching page before line
  -c int
    	concurrency of web requests (default 1)
  -f string
    	obtain patterns from file argument
  -i	ignore cases of input and patterns
  -n	display line number of matching line
  -r	recursively search url directory using links in page
  -s string
    	obtain urls from file argument
  -v	only return non-martching lines
```

## licensing

Can be found [here](LICENSE.txt)
