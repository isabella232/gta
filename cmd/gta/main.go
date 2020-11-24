/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/

// Command gta uses git to find the subset of code changes from a branch
// and then builds the list of go packages that have changed as a result,
// including all dependent go packages.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/digitalocean/gta"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/tools/go/buildutil"
)

// We define this so the tooling works with build tags
func init() {
	flag.Var((*buildutil.TagsFlag)(&build.Default.BuildTags), "tags", buildutil.TagsFlagDoc)
}

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	flagBase := flag.String("base", "origin/master", "base, branch to diff against")
	flagInclude := flag.String("include", "", "define changes to be filtered with a set of comma separated prefixes")
	flagMerge := flag.Bool("merge", false, "diff using the latest merge commit")
	flagJSON := flag.Bool("json", false, "output list of changes as json")
	flagBuildableOnly := flag.Bool("buildable-only", true, "keep buildable changed packages only")
	flagChangedFiles := flag.String("changed-files", "", "path to a file containing a newline separated list of files that have changed")
	flag.Parse()

	if *flagJSON && *flagBuildableOnly {
		log.Fatal("-buildable-only must be set to false when using -json")
	}

	if *flagMerge && len(*flagChangedFiles) > 0 {
		log.Fatal("changed files must not be provided when using the latest merge commit")
	}

	options := []gta.Option{
		gta.SetPrefixes(strings.Split(*flagInclude, ",")...),
	}

	if len(*flagChangedFiles) == 0 {
		// override the differ to use the git differ instead.
		gitDifferOptions := []gta.GitDifferOption{
			gta.SetBaseBranch(*flagBase),
			gta.SetUseMergeCommit(*flagMerge),
		}
		options = append(options, gta.SetDiffer(gta.NewGitDiffer(gitDifferOptions...)))
	} else {
		sl, err := changedFiles(*flagChangedFiles)
		if err != nil {
			log.Fatal(fmt.Errorf("could not read changed file list: %w", err))
		}
		options = append(options, gta.SetDiffer(gta.NewFileDiffer(sl)))
	}

	gt, err := gta.New(options...)
	if err != nil {
		log.Fatalf("can't prepare gta: %v", err)
	}

	packages, err := gt.ChangedPackages()
	if err != nil {
		log.Fatalf("can't list dirty packages: %v", err)
	}

	if *flagJSON {
		err = json.NewEncoder(os.Stdout).Encode(packages)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	strung := stringify(packages.AllChanges, *flagBuildableOnly)

	if terminal.IsTerminal(syscall.Stdin) {
		for _, pkg := range strung {
			fmt.Println(pkg)
		}
		return
	}

	fmt.Println(strings.Join(strung, " "))
}

func stringify(pkgs []*build.Package, validOnly bool) []string {
	var out []string
	for _, pkg := range pkgs {
		if !validOnly || (validOnly && pkg.SrcRoot != "") {
			out = append(out, pkg.ImportPath)
		}
	}
	return out
}

func changedFiles(fn string) ([]string, error) {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	sl := strings.Split(string(b), "\n")
	n := 0
	for _, s := range sl {
		if !keepChangedFile(s) {
			continue
		}

		if !filepath.IsAbs(s) {
			return nil, errors.New("all changed files paths must be absolute paths")
		}

		sl[n] = s
		n++
	}

	return sl[:n], nil
}

func keepChangedFile(s string) bool {
	// Trim spaces, especially in case the newlines were CRLF instead of LF.
	s = strings.TrimSpace(s)

	return len(s) > 0
}
