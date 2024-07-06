# dupescout
A tiny Go package to concurrently find duplicate file paths in a given directory. The determination of whether two files are duplicates can be customized by providing a custom `dupescout.KeyGeneratorFunc` function (see: [key-generator](#key-generator)).

## Installation
```bash
go get github.com/ricci2511/dupescout
```

## Usage
The package exposes two functions: `GetResults` and `StreamResults`. Both take a `dupescout.Cfg` struct to configure the search.

- `GetResults` returns a slice of duplicate file paths once the search is complete. 
- `StreamResults` takes a channel of type `chan []string`, to which it sends each duplicate file path as they are found. Useful if you want to process the results as they come in instead of getting them all at once when the search is complete.

Check out [dedupsc](https://github.com/ricci2511/riccis-homelab-utils/tree/main/dedupsc) for an example on how to use this package. 

```go
package main

import (
    "fmt"
    "github.com/ricci2511/dupescout"
)

func main() {
    filters:= dupescout.Filters{
        HiddenInclude: true,
        DirsExclude: []string{"node_modules"},
        ExtInclude: []string{".txt", ".json", ".go"}, // only search for .txt, .json and .go files
    }
    cfg := dupescout.Cfg{
        Paths: []string{"~/Dev", "~/Documents"},
        Filters: filters,
    }

    fmt.Println("Searching...")

    // Blocks until the search is complete
    dupes := dupescout.GetResults(cfg)

    fmt.Println("Search complete")

    for _, path := range selectedDupes {
        fmt.Println(path)
    }
}
```

The `dupescout.Cfg` struct has the following fields as of now:

```go
type Cfg struct {
	Paths                         // paths to search in for duplicates
	Filters                       // various filters for the search (see filters.go)
	KeyGenerator KeyGeneratorFunc // key generator function to use
	Workers      int              // number of workers (defaults to GOMAXPROCS)
}
```

## key-generator
The `KeyGenerator` field allows you to specify a custom function to generate a key for a given file path that maps to a slice of duplicate file paths.

Some functions are already provided, the default one being `dupescout.Crc32HashKeyGenerator` which simply hashes the first 16KB of file contents with `crc32`. The functions prefixed with `Full` hash the entire file contents instead of just the first 16KB, which is way slower but should be more accurate for rare cases where the first 16KB are not enough. Available `KeyGenerator` functions are:

- `dupescout.Crc32HashKeyGenerator`
- `dupescout.FullCrc32HashKeyGenerator`
- `dupescout.Sha256HashKeyGenerator`
- `dupescout.FullSha256HashKeyGenerator`

In case you want to use custom logic to generate keys, you simply pass a function that satisfies the `dupescout.KeyGeneratorFunc`. An example can be found [here](https://github.com/ricci2511/riccis-homelab-utils/blob/main/dedupsc/movie-tv-key-generator.go).
