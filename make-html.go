package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-tent/tent/core"
	"github.com/russross/blackfriday"
)

var htmlOut = os.Getenv("HTML_OUTDIR")

func MakeHTML() {
	src, err := getSource()
	if err != nil {
		log.Fatalln(err)
	}
	root, err := core.NewRoot(core.Components...)
	if err != nil {
		log.Fatalln(err)
	}
	if err := root.Decode(src); err != nil {
		log.Fatalln(err)
	}
	images := map[[4]string]string{}
	for _, loc := range strings.Split(os.Getenv("HTML_LANGS"), ",") {
		b := bytes.NewBuffer(nil)
		for _, lang := range root.Sub {
			if lang.ID != loc {
				continue
			}
			for _, cat := range lang.Sub {
				if cat.ID == "forms" || cat.ID == "glossary" {
					continue
				}
				fmt.Fprintf(b, "<hr><h1>%s</h1>", cat.Meta["title"])
				for _, sub := range cat.Sub {
					fmt.Fprintf(b, "<hr><h2>%s</h2>", sub.Meta["title"])
					if cat.ID == "tools" {
						addSegments(b, lang, cat, sub, sub, images)
						continue
					}
					for _, dif := range sub.Sub {
						addSegments(b, lang, cat, sub, dif, images)
					}
				}
			}
			if err := ioutil.WriteFile(filepath.Join(htmlOut, loc+".html"), b.Bytes(), 0666); err != nil {
				log.Println(loc, err)
			}
		}
	}
}

func addSegments(b *bytes.Buffer, lang, cat, sub, dif core.Category, images map[[4]string]string) {
	fmt.Fprintf(b, "<hr><h3>%s</h3>", dif.Meta["title"])

	if lang.ID == projectLang {
		for _, cmp := range dif.Components {
			img, ok := cmp.(*core.Picture)
			if !ok {
				continue
			}
			img.ID = path.Base(img.ID)
			images[[4]string{cat.ID, sub.ID, dif.ID, img.ID}] = base64.StdEncoding.EncodeToString(img.Data)
		}
	}

	for _, cmp := range dif.Components {
		seg, ok := cmp.(*core.Segment)
		if !ok {
			check, ok := cmp.(*core.Checks)
			if !ok {
				continue
			}
			fmt.Fprintf(b, "<hr><h3>Checklist</h3><ul>")
			for _, c := range check.List {
				addCheck(b, c, 0)
			}
			fmt.Fprintf(b, "</ul>")
			continue
		}
		seg.Body = imgFinder.ReplaceAllFunc(seg.Body, func(b []byte) []byte {
			name := string(b[bytes.Index(b, []byte{'('})+1 : len(b)-1])
			data := images[[4]string{cat.ID, sub.ID, dif.ID, path.Base(name)}]
			if data == "" {
				log.Println("Cannot find", name, "in", cat.ID, sub.ID, dif.ID, data)
				return b
			}
			return []byte("![image](data:image/png;base64, " + data + ")")
		})
		fmt.Fprintf(b, "<hr><h3>%s</h3>", seg.Meta["title"])
		data := blackfriday.Run(seg.Body)
		b.Write(data)
	}
}

func addCheck(b *bytes.Buffer, c core.Check, deep int) {
	fmt.Fprintf(b, "<li>")
	if c.Check != "" {
		fmt.Fprintf(b, `<input type="checkbox"/> %s`, c.Check)
	} else {
		fmt.Fprintf(b, `<b>%s</b>`, c.Label)
	}
	if len(c.Children) != 0 {
		fmt.Fprintf(b, "<ul>")
		for _, child := range c.Children {
			addCheck(b, child, deep+1)
		}
		fmt.Fprintf(b, "</ul>")
	}
	fmt.Fprintf(b, "</li>")
}
