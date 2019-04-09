package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/russross/blackfriday"

	"github.com/securityfirst/tent/repo"
)

var (
	htmlOut = os.Getenv("HTML_OUTDIR")
)

func MakeHTML() {
	r, err := repo.Local(os.Getenv("TENT_REPODIR"), os.Getenv("TENT_BRANCH"))
	if err != nil {
		log.Fatalf("Repo error: %s", err)
	}
	r.Pull()
	for _, loc := range strings.Split(os.Getenv("HTML_LANGS"), ",") {
		b := bytes.NewBuffer(nil)
		for i, cname := range r.Categories(loc) {
			c := r.Category(cname, loc)
			fmt.Fprintf(b, "<hr><h1>%d. %s</h1>", i+1, c.Name)
			for j, sname := range c.Subcategories() {
				s := c.Sub(sname)
				for _, dname := range s.DifficultyNames() {
					d := s.Difficulty(dname)
					fmt.Fprintf(b, "<hr><h2>%d.%d. %s (%s)</h2>", i+1, j+1, s.Name, d.ID)
					for k, iname := range d.ItemNames() {
						_i := d.Item(iname)
						content := imgFinder.ReplaceAllStringFunc(_i.Body, func(s string) string {

							name := s[strings.Index(s, "(")+1 : len(s)-1]
							a := r.Asset(name)
							if a == nil {
								log.Println("Cannot find", name)
								return s
							}
							return "![image](data:image/png;base64, " + base64.StdEncoding.EncodeToString([]byte(a.Content)) + ")"
						})
						fmt.Fprintf(b, "<hr><h3>%d - %s</h3>", k+1, _i.Title)
						data := blackfriday.Run([]byte(content))
						b.Write(data)
					}
					if c := d.Checks(); c != nil && len(c.Checks) != 0 {
						fmt.Fprintf(b, "<hr><h2>Checklist</h2><hr><ul>")
						for _, check := range c.Checks {
							fmt.Fprintf(b, "<li>%s</li>", check.Text)
						}
						fmt.Fprintf(b, "</ul>")
					}
				}
			}
		}
		if err := ioutil.WriteFile(filepath.Join(htmlOut, loc+".html"), b.Bytes(), 0666); err != nil {
			log.Println(loc, err)
		}
	}
}
