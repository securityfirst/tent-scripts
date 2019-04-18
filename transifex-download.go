package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/go-tent/tent/core"
	"github.com/go-tent/tent/destination"
	"github.com/go-tent/tent/item"
	"github.com/go-tent/tent/source"
	"github.com/go-tent/tent/transifex"
)

func TransifexDownload() {
	langs := os.Args[2:]
	if len(langs) == 0 {
		log.Fatalln("Please specify a language")
	}
	list, err := dstClient.ListResources()
	if err != nil {
		log.Fatalln(err)
	}
	var resources = make(map[string]transifex.Resource)
	for _, r := range list {
		resources[r.Name] = r
	}
	src, err := getSource()
	if err != nil {
		log.Fatalln(err)
	}
	root, err := core.NewRoot(core.Components...)
	if err != nil {
		log.Fatalln(err)
	}
	txsrc := make(transifexSource)
	go txsrc.run(src, resources, langs)
	log.Println("Decoding translations...")
	if err := root.Decode(&txsrc); err != nil {
		log.Fatalln(err)
	}
	for _, cat := range root.Sub {
		for l := range links {
			if err := checkLink(&cat, l); err != nil {
				log.Printf("link: %s %s - %s", cat.ID, l, err)
			}
		}
	}
	dst := destination.NewFile(outDir)
	prefix := make([]string, 0, 4)
	for _, cat := range root.Sub {
		prefix = append(prefix[:1], cat.ID)
		for _, cat := range cat.Sub {
			prefix = append(prefix[:2], cat.ID)
			WriteCat(dst, prefix, &cat)
			for _, cat := range cat.Sub {
				prefix = append(prefix[:3], cat.ID)
				WriteCat(dst, prefix, &cat)
				for _, cat := range cat.Sub {
					prefix = append(prefix[:4], cat.ID)
					WriteCat(dst, prefix, &cat)
				}
			}
		}
	}
}

type msg struct {
	item.Item
	error
}

type transifexSource chan msg

func (t transifexSource) run(src source.Source, resources map[string]transifex.Resource, langs []string) {
	defer close(t)
	for i, err := src.Next(); i != nil; i, err = src.Next() {
		if err != nil {
			t <- msg{nil, err}
			return
		}
		name := strings.TrimPrefix(i.Name(), projectLang+"/")
		if _, ok := i18n[path.Ext(name)]; !ok {
			continue
		}
		r, ok := resources[name]
		if !ok {
			log.Println(name, "not found!")
			continue
		}
		//log.Println(r.Slug)
		for _, l := range langs {
			b, err := dstClient.GetTranslationFile(r.Slug, l)
			if err != nil {

				t <- msg{nil, fmt.Errorf("%s[%s] %s", name, l, err)}
				return
			}
			body := linkFinder.ReplaceAllStringFunc(string(b), replaceLinks)
			t <- msg{item.Memory{ID: "/" + l + "/" + name, Contents: []byte(body)}, nil}
		}
	}
}

func (t transifexSource) Next() (item.Item, error) {
	v := <-t
	return v.Item, v.error
}
