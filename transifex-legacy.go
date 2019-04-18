package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"

	"github.com/go-tent/tent/item"

	"github.com/go-tent/tent/core"
	"github.com/go-tent/tent/source"
	"github.com/go-tent/tent/transifex"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

var resourceMap map[string]*transifex.Resource

const minTranslated = 0.5

var difficultyTx map[string]map[string]string

type Error struct {
	Prefix    []string
	Component core.Component
	Err       error
}

func (e Error) Error() string { return fmt.Sprintf("[%s] %s: %s", e.Prefix, e.Component, e.Err) }

func TransifexLegacy() {
	makeDifficultyTxs()
	resources, err := srcClient.ListResources()
	if err != nil {
		log.Fatalln(err)
	}
	resourceMap = make(map[string]*transifex.Resource, len(resources))
	for i := range resources {
		resourceMap[resources[i].Slug] = &resources[i]
	}
	root, err := makeRoot()
	for _, cat := range root.Sub {
		if cat.ID != projectLang {
			continue
		}
		for _, cat := range cat.Sub {
			if err := handleCat(&cat, nil); err != nil {
				log.Fatalln(err)
			}
		}
		break
	}
}

func handleCat(cat *core.Category, prefix []string) error {
	prefix = append(prefix, cat.ID)
	if len(prefix) != 3 && (cat.ID != "beginner" && cat.ID != "advanced" || cat.ID != "expert") {
		oldSlug := strings.Join(prefix, "_")
		if _, ok := resourceMap[oldSlug]; ok && false {
			handleComponent(prefix, oldSlug, cat, func(cmp core.Component, tx []map[string]string) (core.Component, error) {
				c := *(cmp.(*core.Category))
				c.Meta["title"] = tx[0]["name"]
				return &c, nil
			})
		}
		return handleChildren(cat, prefix)
	}
	key := strings.Join(prefix, "___")
	slug, ok := difficultyTx[key]
	if !ok {
		return handleChildren(cat, prefix)
	}
	result, c := map[string]string{}, *cat
	for lang, tx := range slug {
		c.Meta["description"] = tx
		item, err := core.NewItem(prefix, &c)
		if err != nil {
			log.Printf("\aItem error: %s %s: %s", slug, lang, color.RedString(err.Error()))
			result[lang] = err.Error()
			continue

		}
		if err := uploadTranslation(item, lang); err != nil {
			log.Printf("\aUpload error: %s %s: %s", slug, lang, color.RedString(err.Error()))
			result[lang] = err.Error()
			continue
		}
		result[lang] = "OK"
	}
	log.Println(key, result)
	return handleChildren(cat, prefix)
}

func handleChildren(cat *core.Category, prefix []string) error {
	for _, cmp := range cat.Components {
		switch v := cmp.(type) {
		case *core.Segment:
			break
			handleComponent(prefix, segmentSlug(prefix, v.ID), v, func(cmp core.Component, tx []map[string]string) (core.Component, error) {
				s := *(cmp.(*core.Segment))
				body := tx[0]["body"]
				body = strings.Replace(body, "! [", "![", -1)
				body = strings.Replace(body, ") [", ")[", -1)
				s.Body = []byte(body)
				s.Meta["title"] = tx[0]["title"]
				return &s, nil
			})
		case *core.Checks:
			break
			var slug = strings.Join(prefix, "_") + "__checks"
			original, err := getSrcTranslation(slug, projectLang)
			if err != nil {
				log.Printf("Check src: %s", err)
			}
			handleComponent(prefix, slug, v, func(cmp core.Component, tx []map[string]string) (core.Component, error) {
				checks := *(cmp.(*core.Checks))
				var list = checks.List
				checks.List = make([]core.Check, len(list))
				copy(checks.List, list)
				if got, exp := len(tx), len(checks.List); exp != got {
					if original == nil {
						return nil, fmt.Errorf("Invalid SRC: %s", err)
					}
					log.Println("handling mismatch", slug)
					var m = make(map[string]string, len(original))
					for i := range original {
						m[original[i]["text"]] = tx[i]["text"]
					}
					for i := range checks.List {
						c := &checks.List[i]
						var dst *string
						switch {
						case c.Label != "":
							dst = &c.Label
						case c.Check != "":
							dst = &c.Check
						default:
							return nil, fmt.Errorf("Invalid check: %v", i)
						}
						if trad, ok := m[*dst]; ok {
							*dst = trad
						} else {
							*dst = ""
						}
					}
					return &checks, nil
				}
				for i := range checks.List {
					c := &checks.List[i]
					switch {
					case c.Label != "":
						c.Label = tx[i]["text"]
					case c.Check != "":
						c.Check = tx[i]["text"]
					default:
						return nil, fmt.Errorf("Invalid check: %v", i)
					}
				}
				return &checks, nil
			})
		case *core.Form:
		}
	}
	for _, cat := range cat.Sub {
		if err := handleCat(&cat, prefix); err != nil {
			return err
		}
	}
	return nil
}

func handleComponent(prefix []string, slug string, cmp core.Component, cmpFn func(c core.Component, tx []map[string]string) (core.Component, error)) {
	detail, err := srcClient.ResourceDetail(slug)
	if err != nil {
		if e, ok := err.(transifex.ErrResponse); ok && e.ErrorCode == "not_found" {
			log.Println(slug, color.RedString("NOT_FOUND"))
			return
		}
	}
	var result = map[string]string{}
	for lang, s := range detail.Stats {
		if s["translated"].Percentage < minTranslated {
			continue
		}
		tx, err := getSrcTranslation(slug, lang)
		if err != nil {
			log.Printf("\aTranslation error: %s %s: %s", slug, lang, color.RedString(err.Error()))
			result[lang] = err.Error()
			continue
		}

		v, err := cmpFn(cmp, tx)
		if err != nil {
			log.Printf("\aComponent error: %s %s: %s", slug, lang, color.RedString(err.Error()))
			result[lang] = err.Error()
			continue
		}
		item, err := core.NewItem(prefix, v)
		if err != nil {
			log.Printf("\aItem error: %s %s: %s", slug, lang, color.RedString(err.Error()))
			result[lang] = err.Error()
			continue

		}
		if err := uploadTranslation(item, lang); err != nil {
			log.Printf("\aUpload error: %s %s: %s", slug, lang, color.RedString(err.Error()))
			result[lang] = err.Error()
			continue
		}
		result[lang] = "OK"
	}
	log.Println(slug, result)
}

func segmentSlug(prefix []string, id string) string {
	var slug string
	switch {
	case len(prefix) == 1 && prefix[0] == "glossary":
		slug = fmt.Sprintf("%[1]s___beginner_%[2]s", prefix[0], id)
	case len(prefix) == 2 && prefix[0] == "tools":
		slug = fmt.Sprintf("%[1]s_%[2]s_beginner_%[2]s", prefix[0], id)
	default:
		slug = strings.Join(prefix, "_") + "_" + id
	}
	if id == "what-now" {
		switch strings.TrimSuffix(slug, "_"+id) {
		case "information_malware_beginner", "information_passwords_beginner", "communications_email_beginner",
			"communications_mobile-phones_expert", "communications_radio-and-satellite-phones_expert", "communications_the-internet_beginner",
			"operations_counter-surveillance_beginner", "personal_protective-equipment_beginner", "personal_stress_beginner",
			"operations_protests_beginner", "travel_kidnapping_beginner":
			slug += "-0"
		case "personal_stress_expert", "operations_counter-surveillance_expert", "information_passwords_expert", "travel_kidnapping_expert":
			slug += "-1"
		}
	}
	switch slug {
	case "information_safely-deleting_beginner_secure-deletion-on-mac-os":
		slug = "information_safely-deleting_beginner_secure-deletion-on-mac-os-x"
	case "tools_keepassxc_beginner_keepassxc":
		slug = "tools_keepassx_beginner_keepassx"
	case "tools_orbot-and-orfox_beginner_orbot-and-orfox":
		slug = "tools_orbot-orweb_beginner_orbot-orweb"
	}
	return slug
}

func makeRoot() (*core.Root, error) {
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
	src, err := source.NewGit(context.Background(), c)
	if err != nil {
		return nil, err
	}
	root, err := core.NewRoot(core.Components...)
	if err != nil {
		return nil, err
	}
	if err := root.Decode(src); err != nil {
		return nil, err
	}
	return root, nil
}

func getSrcTranslation(slug, lang string) ([]map[string]string, error) {
	translation, err := srcClient.GetTranslation(slug, lang)
	if err != nil {
		return nil, err

	}
	var m []map[string]string
	if err := json.Unmarshal([]byte(translation["content"].(string)), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func uploadTranslation(item item.Item, lang string) error {
	r, err := item.Content()
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	if _, err := dstClient.UpdateTranslation(makeSlug(item.Name()), lang, string(b)); err != nil {
		return err
	}
	return nil
}

func makeDifficultyTxs() {
	const slug = "difficultiesjson"
	detail, err := srcClient.ResourceDetail(slug)
	if err != nil {
		log.Fatalln(err)
	}
	difficultyTx = make(map[string]map[string]string)
	for lang, s := range detail.Stats {
		if s["translated"].Percentage < minTranslated {
			continue
		}
		translation, err := srcClient.GetTranslation(slug, lang)
		if err != nil {
			log.Fatalln(lang, err)
		}
		var m map[string]string
		if err := json.Unmarshal([]byte(translation["content"].(string)), &m); err != nil {
			log.Fatalln(lang, err)
		}
		for k, v := range m {
			if _, ok := difficultyTx[k]; !ok {
				difficultyTx[k] = make(map[string]string)
			}
			difficultyTx[k][lang] = v
		}
	}
}
