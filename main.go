package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-tent/tent/core"
	"github.com/go-tent/tent/source"
	"github.com/go-tent/tent/transifex"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

const linkPrefix = "umbrella://"

var (
	srcClient   *transifex.Client
	dstClient   *transifex.Client
	projectLang string
	imgFinder   = regexp.MustCompile(`!\[[^\)]*\]\s*\(([^\)]+)\)`)
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

var commit *object.Commit

func getSource() (source.Source, error) {
	if commit == nil {
		log.Println("Cloning...")
		r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{URL: os.Getenv("PROJECT_URL")})
		if err != nil {
			return nil, err
		}
		hash, err := r.Reference(plumbing.ReferenceName("refs/remotes/origin/master"), false)
		if err != nil {
			return nil, err
		}
		if commit, err = r.CommitObject(hash.Hash()); err != nil {
			return nil, err
		}
	}
	log.Println("Parsing...")
	return source.NewGit(context.Background(), commit)
}

var links = make(map[string]struct{})

func checkLink(root *core.Category, link string) error {
	parts := strings.Split(strings.TrimPrefix(link, "umbrella://"), "/")
	cat := root
	for _, p := range parts {
		if path.Ext(p) != "" {
			return nil
		}
		var c *core.Category
		list := make([]string, len(cat.Sub))
		for i := range cat.Sub {
			list[i] = cat.Sub[i].ID
			if cat.Sub[i].ID == p {
				c = &cat.Sub[i]
				break
			}
		}
		if c == nil {
			return fmt.Errorf("cat not found: %s in %s", p, list)
		}
		cat = c
	}
	return nil
}

func replaceLinks(s string) string {
	v, ok := linkFix[s[len(linkPrefix):]]
	if ok {
		s = linkPrefix + v
	}
	links[s] = struct{}{}
	return s
}

var linkFix = map[string]string{
	"lesson/security-planning":                           "assess-your-risk/security-planning",
	"lesson/security-planning/beginner/context":          "assess-your-risk/security-planning/beginner/s_context.md",
	"lesson/phishing/beginner":                           "communications/phishing/beginner",
	"lesson/phishing/how-to-spot-spear-phishing":         "communications/phishing/beginner/s_how-to-spot-spear-phishing.md",
	"lesson/email":                                       "communications/email",
	"lesson/email/1":                                     "communications/email/advanced",
	"lesson/email/0":                                     "communications/email/beginner",
	"lesson/email/2":                                     "communications/email/expert",
	"lesson/making-a-call":                               "communications/making-a-call",
	"lesson/mobile-phones":                               "communications/mobile-phones",
	"lesson/mobile-phones/0":                             "communications/mobile-phones/beginner",
	"lesson/mobile-phones/beginner/burner-phones":        "communications/mobile-phones/beginner/s_burner-phones.md",
	"lesson/mobile-phones/2":                             "communications/mobile-phones/expert",
	"lesson/phishing":                                    "communications/phishing",
	"lesson/radio-and-satellite-phones":                  "communications/radios-and-satellite-phones",
	"lesson/radio-and-satellite-phones/1":                "communications/radios-and-satellite-phones/advanced",
	"lesson/radios-and-satellite-phones/1":               "communications/radios-and-satellite-phones/advanced",
	"lesson/radio-and-satellite-phones/0":                "communications/radios-and-satellite-phones/beginner",
	"lesson/radios-and-satellite-phones/0":               "communications/radios-and-satellite-phones/beginner",
	"lesson/sending-a-message":                           "communications/sending-a-message",
	"lesson/social media":                                "communications/social-media",
	"lesson/social-media":                                "communications/social-media",
	"lesson/social-media/1":                              "communications/social-media/advanced",
	"lesson/social-media/0":                              "communications/social-media/beginner",
	"lesson/social-media/beginner/multimedia":            "communications/social-media/beginner/s_multimedia.md",
	"lesson/social-media/2":                              "communications/social-media/expert",
	"lesson/internet":                                    "communications/the-internet",
	"lesson/the-internet":                                "communications/the-internet",
	"lesson/internet/1":                                  "communications/the-internet/advanced",
	"lesson/the-internet/1":                              "communications/the-internet/advanced",
	"lesson/the-internet/0":                              "communications/the-internet/beginner",
	"lesson/the-internet/2":                              "communications/the-internet/expert",
	"lesson/emergency-support":                           "emergency-support",
	"lesson/emergency-support/digital":                   "emergency-support/digital",
	"forms/digital-security-incident":                    "forms/f_digital-security-incident.yml",
	"forms/proof-life-form":                              "forms/f_proof-life-form.yml",
	"glossary/two-factor-authentication":                 "glossary/s_two-factor-authentication.md",
	"lesson/backing-up":                                  "information/backing-up",
	"lesson/malware":                                     "information/malware",
	"lesson/malware/1":                                   "information/malware/advanced",
	"lesson/malware/0":                                   "information/malware/beginner",
	"lesson/managing-information":                        "information/managing-information",
	"lesson/passwords":                                   "information/passwords",
	"lesson/passwords/0":                                 "information/passwords/beginner",
	"lesson/passwords/1":                                 "information/passwords/advanced",
	"lesson/passwords/2":                                 "information/passwords/expert",
	"lesson/protect-your-workspace":                      "information/protect-your-workspace",
	"lesson/protect-your-workspace/0":                    "information/protect-your-workspace/beginner",
	"lesson/protect-your-workspace/1":                    "information/protect-your-workspace/advanced",
	"lesson/protect-your-workspace/2":                    "information/protect-your-workspace/expert",
	"lesson/protecting-files":                            "information/protecting-files",
	"lesson/protecting-files/1":                          "information/protecting-files/advanced",
	"lesson/safely-deleting":                             "information/safely-deleting",
	"lesson/counter_surveillance/0":                      "incident-response/counter-surveillance/beginner",
	"lesson/counter-surveillance/0":                      "incident-response/counter-surveillance/beginner",
	"lesson/counter_surveillance/1":                      "incident-response/counter-surveillance/advanced",
	"lesson/counter-surveillance/1":                      "incident-response/counter-surveillance/advanced",
	"lesson/counter_surveillance/2":                      "incident-response/counter-surveillance/expert",
	"lesson/counter-surveillance/2":                      "incident-response/counter-surveillance/expert",
	"lesson/arrests":                                     "incident-response/arrests",
	"lesson/arrests/0":                                   "incident-response/arrests/beginner",
	"lesson/arrests/1":                                   "incident-response/arrests/advanced",
	"lesson/arrests/beginner/discrimination-and-torture": "incident-response/arrests/beginner/s_discrimination-and-torture.md",
	"lesson/dangerous-assignments":                       "work/dangerous-assignments",
	"lesson/dangerous-assignments/1":                     "work/dangerous-assignments/advanced",
	"lesson/evacuation":                                  "incident-response/evacuation",
	"lesson/evacuation/0":                                "incident-response/evacuation/beginner",
	"lesson/evacuation/1":                                "incident-response/evacuation/advanced",
	"lesson/meetings":                                    "work/meetings",
	"lesson/protests":                                    "work/protests",
	"lesson/protests/0":                                  "work/protests/beginner",
	"lesson/protests/1":                                  "work/protests/advanced",
	"lesson/public-communications":                       "work/public-communications",
	"lesson/sexual-assault":                              "incident-response/sexual-assault",
	"lesson/sexual-assault/1":                            "incident-response/sexual-assault/advanced",
	"lesson/sexual-assault/0":                            "incident-response/sexual-assault/beginner",
	"lesson/sexual-assault/2":                            "incident-response/sexual-assault/expert",
	"lesson/protective-equipment":                        "travel/protective-equipment",
	"lesson/protective-equipment/1":                      "travel/protective-equipment/advanced",
	"lesson/protective-equipment/0":                      "travel/protective-equipment/beginner",
	"lesson/stress":                                      "stress/stress",
	"lesson/stress/1":                                    "stress/stress/advanced",
	"lesson/stress/0":                                    "stress/stress/beginner",
	"lesson/stress/2":                                    "stress/stress/expert",
	"lesson/encrypt-your-iphone":                         "tools/encryption/s_encrypt-your-iphone.md",
	"lesson/k9-apg":                                      "tools/encryption/s_k9-apg.md",
	"lesson/keepassx":                                    "tools/encryption/s_keepassxc.md",
	"lesson/keepassxc":                                   "tools/encryption/s_keepassxc.md",
	"tools/keepassxc":                                    "tools/encryption/s_keepassxc.md",
	"lesson/cobian-backup":                               "tools/files/s_cobian-backup.md",
	"lesson/recuva":                                      "tools/files/s_recuva.md",
	"lesson/veracrypt":                                   "tools/files/s_veracrypt.md",
	"lesson/mailvelope":                                  "tools/messaging/s_mailvelope.md",
	"lesson/obscuracam":                                  "tools/messaging/s_obscuracam.md",
	"tools/obscuracam":                                   "tools/messaging/s_obscuracam.md",
	"lesson/pidgin":                                      "tools/messaging/s_pidgin.md",
	"lesson/psiphon":                                     "tools/messaging/s_psiphon.md",
	"lesson/signal-for-android":                          "tools/messaging/s_signal-for-android.md",
	"lesson/signal-for-ios":                              "tools/messaging/s_signal-for-ios.md",
	"lesson/signal-for-iOS":                              "tools/messaging/s_signal-for-ios.md",
	"lesson/singal-for-ios":                              "tools/messaging/s_signal-for-ios.md",
	"lesson/android":                                     "tools/other/s_android.md",
	"lesson/facebook":                                    "tools/other/s_facebook.md",
	"lesson/pgp-for-linux":                               "tools/pgp/s_pgp-for-linux.md",
	"lesson/pgp-for-mac-os-x":                            "tools/pgp/s_pgp-for-mac-os-x.md",
	"lesson/pgp-for-windows":                             "tools/pgp/s_pgp-for-windows.md",
	"lesson/orbot-and-orfox":                             "tools/tor/s_orbot-and-orfox.md",
	"lesson/orbot-orfox":                                 "tools/tor/s_orbot-and-orfox.md",
	"lesson/tor-for-linux":                               "tools/tor/s_tor-for-linux.md",
	"lesson/tor-for-mac-os-x":                            "tools/tor/s_tor-for-mac-os-x.md",
	"lesson/tor-for-windows":                             "tools/tor/s_tor-for-windows.md",
	"lesson/borders":                                     "travel/borders",
	"lesson/checkpoints":                                 "travel/checkpoints",
	"lesson/checkpoints/0":                               "travel/checkpoints/beginner",
	"lesson/kidnapping":                                  "incident-response/kidnapping",
	"lesson/kidnapping/1":                                "incident-response/kidnapping/advanced",
	"lesson/kidnapping/0":                                "incident-response/kidnapping/beginner",
	"lesson/kidnapping/2":                                "incident-response/kidnapping/expert",
	"lesson/preparation":                                 "travel/preparation",
	"lesson/vehicles":                                    "travel/vehicles",
	"lesson/vehicles/beginner/drivers-and-vehicles":      "travel/vehicles/beginner/s_drivers-and-vehicles.md",
	"lesson/vehicles/beginner/plan-your-route":           "travel/vehicles/beginner/s_plan-your-route.md",
	"/communications/online-privacy/beginner/multimedia": "/communications/online-privacy/beginner/s_multimedia.md",
}
