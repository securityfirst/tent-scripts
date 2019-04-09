package main

import (
	"io"
	"io/ioutil"
	"log"
	"path"
	"strings"

	"github.com/go-tent/tent/transifex"
)

var deleteResources bool

func TransifexUpload() {
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
	var errors [][2]string
	for item, err := src.Next(); item != nil; item, err = src.Next() {
		if err != nil {
			log.Fatalln(err)
		}

		var (
			name = strings.TrimPrefix(item.Name(), projectLang+"/")
			ext  = path.Ext(name)
		)
		if !strings.HasPrefix(name, "forms") {
			continue
		}
		if _, ok := i18n[ext]; !ok {
			continue
		}

		r, err := item.Content()
		if err != nil {
			log.Fatalf("[%s] Error: %s.", name, err)
		}

		if err := handleItem(name, ext, r, resources); err != nil {
			log.Printf("[%s] Error: %s.", name, err)
			errors = append(errors, [2]string{name, err.Error()})
			continue
		}
		log.Printf("[%s] ok.", name)
	}
	if len(resources) != 0 {
		log.Printf("*** Extra Resources ***")
		for _, r := range resources {
			if deleteResources {
							if err := dstClient.DeleteResource(r.Slug); err != nil {
					log.Printf("[%s] Error: %s.", r.Slug, err)
					continue
				}
				log.Printf("[%s] deleted.", r.Slug)
			}
		}
	}

	if len(errors) != 0 {
		log.Printf("*** Errors ***")
		for _, e := range errors {
			log.Printf("[%s] %s.", e[0], e[1])
		}
	}
}

func handleItem(name, ext string, r io.ReadCloser, resources map[string]transifex.Resource) error {
	defer r.Close()
	contents, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	slug := makeSlug(name)
	if _, ok := resources[name]; !ok {
			_, err = dstClient.CreateResource(transifex.UploadResourceRequest{
			BaseResource:       transifex.BaseResource{Slug: slug, Name: name, I18nType: i18n[ext]},
			AcceptTranslations: true,
			Content:            string(contents),
		})
		if err != nil {
			return err
		}
	} else {
		delete(resources, name)
	}
	for _, l := range []KeyLocker{catLocker{}, formLocker{}} {
		if l.SkipFile(name) {
			continue
		}
			strs, err := dstClient.GetStrings(slug, projectLang)
		if err != nil {
			return err
		}
		for _, s := range strs {
					log.Println(s.Key, l.KeyTags(s.Key))
			if err := dstClient.SetStringTags(slug, s.StringHash, l.KeyTags(s.Key)...); err != nil {
				return err
			}
		}
	}
	return nil
}

type KeyLocker interface {
	SkipFile(name string) bool
	KeyTags(key string) []string
}

type formLocker struct{}

func (formLocker) SkipFile(name string) bool {
	return !strings.HasPrefix(name, "forms/")
}

func (formLocker) KeyTags(key string) (tags []string) {
	p := strings.Split(key, ".")
	if key := p[len(p)-1]; key == "label" || key == "title" {
		return tags
	}
	return []string{"locked"}
}

type catLocker struct{}

func (catLocker) SkipFile(name string) bool {
	return !strings.HasSuffix(name, "/.category.yml")
}

func (catLocker) KeyTags(key string) (tags []string) {
	if key != "icon" {
		return tags
	}
	return []string{"locked"}
}
