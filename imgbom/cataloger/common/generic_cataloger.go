package common

import (
	"strings"

	"github.com/anchore/imgbom/imgbom/pkg"
	"github.com/anchore/imgbom/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
)

// TODO: put under test...

// GenericCataloger implements the Catalog interface and is responsible for dispatching the proper parser function for
// a given path or glob pattern. This is intended to be reusable across many package cataloger types.
type GenericCataloger struct {
	globParsers   map[string]ParserFn
	pathParsers   map[string]ParserFn
	selectedFiles []file.Reference
	parsers       map[file.Reference]ParserFn
}

// NewGenericCataloger if provided path-to-parser-function and glob-to-parser-function lookups creates a GenericCataloger
func NewGenericCataloger(pathParsers map[string]ParserFn, globParsers map[string]ParserFn) GenericCataloger {
	return GenericCataloger{
		globParsers:   globParsers,
		pathParsers:   pathParsers,
		selectedFiles: make([]file.Reference, 0),
		parsers:       make(map[file.Reference]ParserFn),
	}
}

// register pairs a set of file references with a parser function for future cataloging (when the file contents are resolved)
func (a *GenericCataloger) register(files []file.Reference, parser ParserFn) {
	a.selectedFiles = append(a.selectedFiles, files...)
	for _, f := range files {
		a.parsers[f] = parser
	}
}

// clear deletes all registered file-reference-to-parser-function pairings from former SelectFiles() and register() calls
func (a *GenericCataloger) clear() {
	a.selectedFiles = make([]file.Reference, 0)
	a.parsers = make(map[file.Reference]ParserFn)
}

// SelectFiles takes a set of file trees and resolves and file references of interest for future cataloging
func (a *GenericCataloger) SelectFiles(trees []tree.FileTreeReader) []file.Reference {
	for _, t := range trees {
		// select by exact path
		for path, parser := range a.pathParsers {
			f := t.File(file.Path(path))
			if f != nil {
				a.register([]file.Reference{*f}, parser)
			}
		}

		// select by pattern
		for globPattern, parser := range a.globParsers {
			fileMatches, err := t.FilesByGlob(globPattern)
			if err != nil {
				log.Errorf("failed to find files by glob: %s", globPattern)
			}
			if fileMatches != nil {
				a.register(fileMatches, parser)
			}
		}
	}

	return a.selectedFiles
}

// Catalog takes a set of file contents and uses any configured parser functions to resolve and return discovered packages
func (a *GenericCataloger) Catalog(contents map[file.Reference]string, upstreamMatcher string) ([]pkg.Package, error) {
	defer a.clear()

	packages := make([]pkg.Package, 0)

	for reference, parser := range a.parsers {
		content, ok := contents[reference]
		if !ok {
			log.Errorf("cataloger '%s' missing file content: %+v", upstreamMatcher, reference)
			continue
		}

		entries, err := parser(strings.NewReader(content))
		if err != nil {
			log.Errorf("cataloger '%s' failed to parse entries (reference=%+v): %w", upstreamMatcher, reference, err)
			continue
		}

		for _, entry := range entries {
			entry.FoundBy = upstreamMatcher
			entry.Source = []file.Reference{reference}

			packages = append(packages, entry)
		}
	}

	return packages, nil
}