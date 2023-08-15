package dupescout

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/puzpuzpuz/xsync/v2"
	"golang.org/x/sync/errgroup"
)

type pair struct {
	key  string // depends on the KeyGeneratorFunc
	path string
}

// Dupescout is the main struct that holds the state of the search.
type dupescout struct {
	g        *errgroup.Group
	pairs    chan *pair
	shutdown chan os.Signal
}

func newDupeScout(workers int) *dupescout {
	g := new(errgroup.Group)
	g.SetLimit(workers)

	return &dupescout{
		g:        g,
		pairs:    make(chan *pair, 500),
		shutdown: make(chan os.Signal, 1),
	}
}

// Starts the search for duplicates which can be customized by the provided Cfg struct.
func run(c Cfg, resultsChan chan<- []string, dupesChan chan<- string) error {
	c.defaults()
	dup := newDupeScout(c.Workers)

	if dupesChan != nil {
		// Stream results to dupesChan as duplicates are found.
		go dup.consumePairs(nil, dupesChan)
	} else if resultsChan != nil {
		// Sends all duplicates to the resultsChan once the search is done.
		go dup.consumePairs(resultsChan, nil)
	} else {
		// Sanity check.
		return fmt.Errorf("either resultsChan or dupesChan must be provided")
	}

	go gracefulShutdown(dup.shutdown)
	dup.g.Go(func() error {
		return dup.search(c.Path, &c)
	})

	err := dup.g.Wait()
	close(dup.pairs) // Trigger the pair consumer to process the results.
	return err
}

// Runs the duplicate search and returns the results once the search is done (blocking).
func GetResults(c Cfg) ([]string, error) {
	results := make(chan []string, 1)
	err := run(c, results, nil)
	return <-results, err
}

// Runs the duplicate search and streams the results to the provided channel as they are
// found (non-blocking).
//
// Must run in a separate goroutine to avoid blocking the main thread.
func StreamResults(c Cfg, dupesChan chan<- string) error {
	return run(c, nil, dupesChan)
}

// Processes the pairs, and depending on the provided arguments, it will either send the
// results to the results channel once all pairs have been processed, or it will stream
// each encountered duplicate path to the dupesChan channel.
//
// When streaming, the results channel is ignored since all results have been sent to dupesChan.
func (dup *dupescout) consumePairs(results chan<- []string, dupesChan chan<- string) {
	streaming := dupesChan != nil // stream paths when dupesChan is provided
	m := xsync.NewMapOf[[]string]()

	for p := range dup.pairs {
		paths, ok := m.Load(p.key)
		if ok {
			m.Store(p.key, append(paths, p.path))
			if streaming {
				// Also send the fist path if it hasn't been sent yet.
				if len(paths) == 1 {
					dupesChan <- paths[0]
				}
				dupesChan <- p.path
			}
		} else {
			m.Store(p.key, []string{p.path})
		}
	}

	if streaming {
		close(dupesChan)
		return
	}

	results <- processResults(m)
}

// Produces a pair with the key which is generated by the KeyGeneratorFunc and the path
// which is then sent to the pairs channel.
func (dup *dupescout) producePair(path string, keyGen KeyGeneratorFunc) error {
	// Stop pair production if a shutdown signal has been received.
	if dup.shuttingDown() {
		return nil
	}

	key, err := keyGen(path)
	if err != nil {
		return err
	}

	if key == "" {
		return fmt.Errorf("key generator returned an empty key for path: %s", path)
	}

	dup.pairs <- &pair{key, path}
	return nil
}

// Walks the tree of the provided dir and triggers the production of pairs for each valid file.
func (dup *dupescout) search(dir string, c *Cfg) error {
	return filepath.WalkDir(dir, func(path string, de os.DirEntry, err error) error {
		// Stop searching if a shutdown signal has been received.
		if dup.shuttingDown() {
			return nil
		}

		if err != nil {
			return err
		}

		if de.IsDir() && c.skipDir(path) {
			return filepath.SkipDir
		}

		if de.Type().IsRegular() && !c.skipFile(path) {
			fi, err := de.Info()
			if err != nil || fi.Size() == 0 {
				return nil
			}

			dup.g.Go(func() error {
				return dup.producePair(path, c.KeyGenerator)
			})
		}

		return nil
	})
}

// Processes a map of keys to paths and returns a slice of paths that are duplicates.
func processResults(m *xsync.MapOf[string, []string]) []string {
	res := []string{}

	m.Range(func(key string, paths []string) bool {
		if len(paths) > 1 {
			res = append(res, paths...)
		}

		return true
	})

	return res
}

// Helper to check if a shutdown signal has been received.
func (dup *dupescout) shuttingDown() bool {
	select {
	case <-dup.shutdown:
		return true
	default:
		return false
	}
}

// Sets up a signal handler worker for graceful shutdown.
func gracefulShutdown(shutdown chan os.Signal) {
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown
	log.Println("\nReceived signal, shutting down after current workers are done...")
	close(shutdown)
}
