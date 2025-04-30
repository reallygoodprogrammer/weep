# weep (web-e grep)

A go program for greping webpages with my most used grep options.

## usage

```
usage: ./weep <pattern> [url(s) or will read stdin]
-> ctrl-c to stop recursive greping
  -H	display URL of matching page before line
  -c int
    	concurrency of web requests (default 10) (default 10)
  -d string
    	obtain allowed domains to search from file argument
  -f string
    	obtain patterns from file argument
  -i	ignore cases of input and patterns
  -l float
    	rate of requests per second (default: none) (default 0.5)
  -n	display line number of matching line
  -o string
    	output file name to write matches too
  -s	do not recursively search for new pages (single request)
  -v	only return non-martching lines
```

## licensing

Can be found [here](LICENSE.txt)
