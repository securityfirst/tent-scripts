package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/go-tent/tent/core"
	"github.com/go-tent/tent/destination"
	"github.com/securityfirst/tent/repo"
)

var (
	outDir  = os.Getenv("TENT_OUTDIR")
	repoDir = os.Getenv("TENT_REPODIR")
	branch  = os.Getenv("TENT_BRANCH")
	option  = struct {
		SplitTools, SplitGlossary bool
	}{true, false}
)

func GitParse() {
	r, err := repo.Local(repoDir, branch)
	if err != nil {
		log.Fatalf("Repo error: %s", err)
	}
	r.Pull()
	for _, loc := range []string{"en"} {
		root, err := CreateRoot(loc, r)
		if err != nil {
			log.Fatal(loc, err)
		}
		dir := path.Join(outDir, loc)
		os.RemoveAll(path.Join(outDir, loc))
		dst := destination.NewFile(dir)
		prefix := make([]string, 0, 3)
		for _, cat := range root.Sub {
			prefix = append(prefix[:1], cat.ID)
			WriteCat(dst, prefix, &cat)
			for _, cat := range cat.Sub {
				prefix = append(prefix[:2], cat.ID)
				WriteCat(dst, prefix, &cat)
				for _, cat := range cat.Sub {
					prefix = append(prefix[:3], cat.ID)
					WriteCat(dst, prefix, &cat)
				}
			}
		}
	}
}

var diffOrder = map[string]float64{
	"beginner": 1, "advanced": 2, "expert": 3,
}

func CreateRoot(loc string, r *repo.Repo) (*core.Root, error) {
	root, _ := core.NewRoot(&core.Checks{}, &core.Form{})
	for i, cname := range r.Categories(loc) {
		c := r.Category(cname, loc)
		cat := core.Category{
			ID:    c.ID,
			Index: float64(i) + 1,
			Meta: map[string]string{
				"title": c.Name,
			},
		}
		addIcon(&cat, nil)
		switch cat.ID {
		case "glossary":
			cat.Meta["template"] = "glossary"
		}
		for i, sname := range c.Subcategories() {
			s := c.Sub(sname)
			sub := core.Category{
				ID:    s.ID,
				Index: float64(i) + 1,
				Meta: map[string]string{
					"title": s.Name,
				},
			}
			for _, dname := range s.DifficultyNames() {
				d := s.Difficulty(dname)
				diff := core.Category{
					ID:    d.ID,
					Index: diffOrder[d.ID],
					Meta: map[string]string{
						"title":       strings.Title(d.ID),
						"description": d.Descr,
					},
				}
				for _i, iname := range d.ItemNames() {
					i := d.Item(iname)
					if strings.HasSuffix(iname, "-0") || strings.HasSuffix(iname, "-1") {
						iname = iname[:len(iname)-2]
					}
					i.Body = strings.Replace(i.Body, "] (", "](", -1)
					i.Body = linkFinder.ReplaceAllStringFunc(i.Body, replaceLinks)
					seg := core.Segment{
						ID:    iname,
						Index: float64(_i) + 1,
						Meta: map[string]string{
							"title": i.Title,
						},
						Body: []byte(i.Body),
					}
					if cat.ID == "tools" || cat.ID == "glossary" {
						seg.Index = 0
					}
					diff.Components = append(diff.Components, &seg)
					diff.Components = append(diff.Components, getPics(r, i.ID, i.Body)...)
				}
				if c := d.Checks(); c != nil && len(c.Checks) != 0 {
					var checks = core.Checks{ID: "checklist", Index: 100}
					for _, c := range c.Checks {
						c.Text = linkFinder.ReplaceAllStringFunc(c.Text, replaceLinks)
						var check core.Check
						if c.NoCheck {
							check.Label = c.Text
						} else {
							check.Check = c.Text
						}
						checks.List = append(checks.List, check)
					}
					diff.Components = append(diff.Components, &checks)
				}
				sub.Sub = append(sub.Sub, diff)
			}
			switch cat.ID {
			case "about":
				cat.Components = append(cat.Components, sub.Sub[0].Components...)
			case "tools":
				if option.SplitTools {
					splitTools(&cat, &sub)
					break
				}
				v := sub.Sub[0].Components[0].(*core.Segment)
				v.Meta = sub.Meta
				v.Index = sub.Index
				cat.Components = append(cat.Components, sub.Sub[0].Components...)
			case "glossary":
				if option.SplitGlossary {
					splitGlossary(&cat, &sub)
					break
				}
				idx := 1.0
				for _, c := range sub.Sub[0].Components {
					if s, ok := c.(*core.Segment); ok {
						s.Index = idx
						idx++
					}
					cat.Components = append(cat.Components, sub.Sub[0].Components...)

				}
			default:
				cat.Sub = append(cat.Sub, sub)
			}
		}
		root.Sub = append(root.Sub, cat)
	}
	root.Sub = append(root.Sub, getForms(r, loc))
	for l := range links {
		if err := checkLink(root.Category, l); err != nil {
			log.Printf("link: %s - %s", l, err)
		}

	}
	return root, nil
}

func splitTools(cat, sub *core.Category) {
	v := sub.Sub[0].Components[0].(*core.Segment)
	v.Meta = sub.Meta
	v.Index = sub.Index
	if len(cat.Sub) == 0 {
		cat.Sub = []core.Category{
			{Index: 1, ID: "messaging", Meta: map[string]string{"title": "Messaging"}},
			{Index: 2, ID: "encryption", Meta: map[string]string{"title": "Encryption"}},
			{Index: 3, ID: "pgp", Meta: map[string]string{"title": "PGP"}},
			{Index: 4, ID: "tor", Meta: map[string]string{"title": "Tor"}},
			{Index: 5, ID: "files", Meta: map[string]string{"title": "Files"}},
			{Index: 6, ID: "other", Meta: map[string]string{"title": "Other"}},
		}
	}

	var idx int
	switch v.ID {
	case "mailvelope", "obscuracam", "pidgin", "psiphon", "signal-for-android", "signal-for-ios":
		idx = 0
	case "encrypt-your-iphone", "k9-apg", "keepassxc":
		idx = 1
	case "pgp-for-linux", "pgp-for-mac-os-x", "pgp-for-windows":
		idx = 2
	case "tor-for-linux", "tor-for-mac-os-x", "tor-for-windows", "orbot-and-orfox":
		idx = 3
	case "cobian-backup", "recuva", "veracrypt":
		idx = 4
	case "android", "facebook":
		idx = 5
	default:
		panic(v.ID)
	}
	cat.Sub[idx].Components = append(cat.Sub[idx].Components, sub.Sub[0].Components...)
}

func splitGlossary(cat, sub *core.Category) {
	cat.Sub = []core.Category{
		{Index: 1, ID: "a-d", Meta: map[string]string{"title": "A-D"}},
		{Index: 2, ID: "e-h", Meta: map[string]string{"title": "E-H"}},
		{Index: 3, ID: "i-l", Meta: map[string]string{"title": "I-L"}},
		{Index: 4, ID: "m-p", Meta: map[string]string{"title": "M-P"}},
		{Index: 5, ID: "q-t", Meta: map[string]string{"title": "Q-T"}},
		{Index: 6, ID: "u-z", Meta: map[string]string{"title": "U-Z"}},
	}
	for _, cmp := range sub.Sub[0].Components {
		v := cmp.(*core.Segment)
		var idx int
		switch {
		case v.ID[0] >= 'a' && v.ID[0] <= 'd':
			idx = 0
		case v.ID[0] >= 'e' && v.ID[0] <= 'h':
			idx = 1
		case v.ID[0] >= 'i' && v.ID[0] <= 'l':
			idx = 2
		case v.ID[0] >= 'm' && v.ID[0] <= 'p':
			idx = 3
		case v.ID[0] >= 'q' && v.ID[0] <= 't':
			idx = 4
		case v.ID[0] >= 'u' && v.ID[0] <= 'z':
			idx = 5
		default:
			panic(v.ID)
		}
		v.Index = float64(len(cat.Sub[idx].Components) + 1)
		cat.Sub[idx].Components = append(cat.Sub[idx].Components, v)
	}

}

func getForms(r *repo.Repo, loc string) core.Category {
	cat := core.Category{
		ID: "forms",
	}
	for _, fname := range r.Forms(loc) {
		f := r.Form(fname, loc)
		var form = core.Form{ID: fname, Meta: map[string]string{"title": f.Name}}
		for _, s := range f.Screens {
			screen := core.FormScreen{Meta: core.Map{"title": s.Name}}
			for _, i := range s.Items {
				t := i.Type
				item := core.FormItem{
					Name: i.Name,
					Type: t,
					Meta: core.Map{},
				}
				if i.Options != nil {
					options := make([]interface{}, 0, len(i.Options))
					for _, o := range i.Options {
						o = strings.TrimSpace(o)
						options = append(options, map[string]string{"value": o, "label": o})
					}
					item.Meta["options"] = options
				}
				if i.Lines != 0 {
					item.Meta["lines"] = i.Lines
				}
				if i.Hint != "" {
					item.Meta["hint"] = i.Hint
				}
				if i.Label != "" {
					item.Meta["label"] = i.Label
				}
				screen.Items = append(screen.Items, item)
			}
			form.Screens = append(form.Screens, screen)
		}
		cat.Components = append(cat.Components, &form)
	}
	return cat
}

func addIcon(cat, parent *core.Category) {
	p := cat.ID + ".png"
	if parent != nil {
		p = parent.ID + "_" + p
	}
	b, err := ioutil.ReadFile(path.Join("icons", p))
	if err != nil {
		log.Println("! icon not found", p)
		return
	}
	pic := core.Picture{ID: path.Base(p), Data: b}
	cat.Meta["icon"] = pic.ID
	cat.Components = append(cat.Components, &pic)
}

func getPics(r *repo.Repo, id, contents string) []core.Component {
	var done = map[string]struct{}{}
	var list []core.Component
	for _, match := range imgFinder.FindAllStringSubmatch(contents, -1) {
		picName := path.Base(match[1])
		if _, ok := done[picName]; ok {
			continue
		}
		done[picName] = struct{}{}
		ass := r.Asset(picName)
		if ass == nil {
			log.Println("! image not found", id, picName)
			continue
		}
		list = append(list, &core.Picture{ID: picName, Data: []byte(ass.Content)})
	}
	if l := len(list); l != 0 || id == "signal-for-ios" {
	}
	return list
}

func WriteCat(dst destination.Destination, prefix []string, cat *core.Category) {
	item, err := core.NewItem(prefix, cat)
	if err != nil {
		log.Fatal(prefix, err)
	}
	if err := dst.Create(context.Background(), item); err != nil {
		log.Fatal(prefix, item, err)
	}
	count := .0
	for _, cmp := range cat.Components {
		if s, ok := cmp.(*core.Segment); ok && s.Index == 0 {
			count++
			s.Index = count
		}
		item, err := core.NewItem(prefix, cmp)
		if err != nil {
			log.Fatal(prefix, cmp, err)
		}
		if err := dst.Create(context.Background(), item); err != nil {
			if !os.IsExist(err) {
				log.Fatal(prefix, item, cmp, err)
			}
		}
	}
}
