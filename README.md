# weep (web-e grep)

A go program for greping webpages with my most used grep options.

## usage

```
usage: cat <urls-file> | ./wepp <pattern> [url+]
  -H	display URL of matching page before line
  -c int
    	concurrency of web requests (default 10) (default 10)
  -d string
    	obtain allowed domains to search from file argument
  -f string
    	obtain patterns from file argument
  -i	ignore cases of input and patterns
  -l float
    	rate of requests per second (default 0.5 sec) (default 0.5)
  -n	display line number of matching line
  -r	recursively search using src & href values
  -v	only return non-martching lines
```

## licensing

Can be found [here](LICENSE.txt)
