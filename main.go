package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"github.com/roffe/rssbot/webhook"
	"gopkg.in/yaml.v2"
)

var (
	debug      = false
	config     *RuntimeConfig
	configFile string
)

// used for semaphore chan
type token struct{}

// RuntimeConfig file struct
type RuntimeConfig struct {
	Config     *Config          `yaml:"config"`
	Feeds      map[string]*Feed `yaml:"feeds"`
	sync.Mutex `yaml:"-"`
}

// Config is the app conf
type Config struct {
	MaxConcurrency int `yaml:"maxConcurrency"`
}

// Feed config
type Feed struct {
	Hooks         []string      `yaml:"discordHooks"`
	Color         string        `yaml:"color"`
	LastPublished time.Time     `yaml:"lastPublished"`
	LastRun       time.Time     `yaml:"lastRun"`
	LastUpdated   time.Time     `yaml:"lastUpdate"`
	Periode       time.Duration `yaml:"periode"`
	URL           string        `yaml:"url"`
}

func loadConfig() {
	f, err := os.Open(configFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	d := yaml.NewDecoder(f)
	if err := d.Decode(&config); err != nil {
		log.Fatal(err)
	}
}

func saveConfig() {
	if debug {
		log.Println("saving config")
	}
	config.Lock()
	defer config.Unlock()

	f, err := os.OpenFile(configFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	d := yaml.NewEncoder(f)
	defer d.Close()

	if err := d.Encode(config); err != nil {
		log.Fatal(err)
	}
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&configFile, "config", "config.yml", "specify config file")
	flag.Parse()
	webhook.Debug = debug
	loadConfig()
	log.Println("RSS Scraper starting")
	log.Println("maxConcurrency:", config.Config.MaxConcurrency)
	if debug {
		log.Println("debug mode enabled")
	}
}

func main() {
	defer saveConfig()

	save := time.NewTicker(10 * time.Second)
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func(c context.CancelFunc) {
		for {
			select {
			case sig := <-sigChan:
				log.Println("got signal", sig)
				cancel()
			}
		}
	}(cancel)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			runFeeds(ctx)
		case <-save.C:
			saveConfig()
		}

	}
}

func runFeeds(ctx context.Context) {
	sem := make(chan token, config.Config.MaxConcurrency)
	wg := sync.WaitGroup{}
	config.Lock()
	defer config.Unlock()
	for name, feed := range config.Feeds {
		select {
		case <-ctx.Done():
			log.Println("aborted feed sync due to", ctx.Err())
			return
		default:
		}
		if time.Since(feed.LastRun) < feed.Periode {
			//fmt.Println("Skipping, Time since last run ", time.Since(feed.LastRun))
			continue
		}
		sem <- token{}
		wg.Add(1)
		go processFeed(ctx, name, feed, sem, &wg)
	}
	wg.Wait()
}

var nl = regexp.MustCompile(`\n{3,}`)

func processFeed(ctx context.Context, name string, feed *Feed, sem chan token, wg *sync.WaitGroup) {
	defer func() {
		<-sem
		wg.Done()
	}()

	if debug {
		log.Printf("fetching %s - %s\n", name, feed.URL)
	}

	ctx2, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	rssFeed, err := gofeed.NewParser().ParseURLWithContext(feed.URL, ctx2)
	if err != nil {
		log.Println(err)
		return
	}
	p := bluemonday.StrictPolicy()
	sort.Sort(rssFeed)
	if feed.LastUpdated.Unix() < rssFeed.UpdatedParsed.Unix() {
		for _, news := range rssFeed.Items {
			if feed.LastPublished.Unix() < news.PublishedParsed.Unix() {
				feed.LastPublished = *news.PublishedParsed
				log.Println(news.Title, news.Published)
				r := strings.NewReader(news.Description)
				doc, err := goquery.NewDocumentFromReader(r)
				if err != nil {
					log.Println(err)
					continue
				}
				var image string
				doc.Find("img").EachWithBreak(func(i int, s *goquery.Selection) bool {
					img, exists := s.Attr("src")
					if exists {
						image = img
					}
					return false
				})

				desc := p.Sanitize(news.Description)
				desc = strings.ReplaceAll(desc, "\t", "")
				desc = nl.ReplaceAllString(desc, "\n")
				cap := 500
				if len(desc) < 500 {
					cap = len(desc)
				}
				for _, hookURL := range feed.Hooks {
					msg := webhook.NewMessage(hookURL, true)
					e := &webhook.Embed{
						Title:       news.Title,
						Type:        webhook.TypeRich,
						Description: desc[0:cap] + "...",
						URL:         news.Link,
						Color:       webhook.Hex2int(feed.Color),
						Timestamp:   news.PublishedParsed,
						Thumbnail: &webhook.EmbedThumbnail{
							URL:    "https://static.mmo-champion.com/images/tranquilizing/logo.png",
							Width:  157,
							Height: 90,
						},
						Author: &webhook.EmbedAuthor{
							Name:    "By Purple Haze",
							URL:     "https://purplehazeeu.com",
							IconURL: "https://purplehazeeu.com/wp/wp-content/uploads/2020/09/ph-logo-smal.png",
						},
					}
					if image != "" {
						e.Image = &webhook.EmbedImage{
							URL: image,
						}
					}

					if news.Image != nil {
						e.Thumbnail = &webhook.EmbedThumbnail{
							URL: news.Image.URL,
						}
					}
					msg.AddEmbed(e)
					if err := msg.Send(); err != nil {
						log.Print(err)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
		feed.LastUpdated = *rssFeed.UpdatedParsed
	}
	/*
		if feed.LastUpdated.Unix() < rssFeed.UpdatedParsed.Unix() {
			if last := len(rssFeed.Items) - 1; last >= 0 {
				for i, news := last, rssFeed.Items[0]; i >= 0; i-- {
					news = rssFeed.Items[i]
					if feed.LastEntry.Unix() < news.PublishedParsed.Unix() {
						feed.LastEntry = *news.PublishedParsed
						log.Println(news.Title, news.Published)
						msg := webhook.NewMessage(feed.Hook, true)
						e := &webhook.Embed{
							Title: news.Title,
							// Description: "Read it at MMO-Champion",
							URL:       news.Link,
							Color:     webhook.Hex2int(feed.Color),
							Timestamp: news.PublishedParsed,
							Thumbnail: &webhook.EmbedThumbnail{
								URL:    "https://static.mmo-champion.com/images/tranquilizing/logo.png",
								Width:  157,
								Height: 90,
							},
						}
						if news.Image != nil {
							e.Thumbnail = &webhook.EmbedThumbnail{
								URL: news.Image.URL,
							}
						}
						msg.AddEmbed(e)
						if err := msg.Send(); err != nil {
							log.Print(err)
						}
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
			feed.LastUpdated = *rssFeed.UpdatedParsed
		}

	*/
	feed.LastRun = time.Now()
}
