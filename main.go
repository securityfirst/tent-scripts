package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-tent/tent/source"
	"github.com/go-tent/tent/transifex"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

const linkPrefix = "umbrella://"

var (
	srcClient   *transifex.Client
	dstClient   *transifex.Client
	projectLang string
	imgFinder   = regexp.MustCompile(`!\[.[^\)]*\]\(([^\)]+)\)`)
	linkFinder  = regexp.MustCompile(linkPrefix + `[^)]*`)
)

var (
	i18n    = map[string]string{".yml": `YAML_GENERIC`, ".md": `GITHUBMARKDOWN`}
	slugger = map[rune]rune{'/': '_', '.': '_', ' ': '-', '\'': '-'}
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	projectLang = os.Getenv("TX_PROJ_LANG")
	srcClient = transifex.NewClient(os.Getenv("TX_API_KEY"), os.Getenv("TX_SRC_ORG"), os.Getenv("TX_SRC_PROJ"))
	dstClient = transifex.NewClient(os.Getenv("TX_API_KEY"), os.Getenv("TX_DST_ORG"), os.Getenv("TX_DST_PROJ"))
	t := time.NewTicker(time.Hour / 6000)
	srcClient.SetTicker(t)
	dstClient.SetTicker(t)

}

func main() {
	var options = map[string]func(){
		"git-parse":          GitParse,
		"make-html":          MakeHTML,
		"transifex-legacy":   TransifexLegacy,
		"transifex-upload":   TransifexUpload,
		"transifex-download": TransifexDownload,
	}
	if len(os.Args) == 1 || options[os.Args[1]] == nil {
		b := bytes.NewBuffer(nil)
		fmt.Fprint(b, "Please choose one of:")
		for o := range options {
			fmt.Fprintf(b, " %q", o)
		}
		log.Println(b.String())
		os.Exit(127)
	}
	options[os.Args[1]]()
}

func makeSlug(s string) string {
	return strings.Map(func(r rune) rune {
		v, ok := slugger[r]
		if !ok {
			return r
		}
		return v
	}, s)
}

func getSource() (source.Source, error) {
	log.Println("Cloning...")
	r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{URL: os.Getenv("PROJECT_URL")})
	if err != nil {
		return nil, err
	}
	hash, err := r.Reference(plumbing.ReferenceName("refs/remotes/origin/master"), false)
	if err != nil {
		return nil, err
	}
	c, err := r.CommitObject(hash.Hash())
	if err != nil {
		return nil, err
	}
	log.Println("Parsing...")
	return source.NewGit(context.Background(), c)
}
