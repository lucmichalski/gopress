package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"io"
	"time"
	"net/url"
	"unicode/utf8"

	// "sort"
	"net/http"
	"path/filepath"
	"regexp"

	// "mvdan.cc/xurls/v2"
	// "github.com/h2non/filetype"
	// "github.com/jinzhu/now"

	// "github.com/gosimple/slug"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"

	"github.com/nozzle/throttler"
	"github.com/corpix/uarand"
	badger "github.com/dgraph-io/badger"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/queue"
	"github.com/golang/snappy"
	"github.com/google/go-github/v29/github"
	"github.com/iancoleman/strcase"
	"github.com/joho/godotenv"
	"github.com/k0kubun/pp"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"github.com/x0rzkov/go-vcsurl"
	// "github.com/gomarkdown/markdown"
	"github.com/lucmichalski/gopress/pkg/models"

	ghclient "github.com/lucmichalski/gopress/pkg/client"
)

var (
	clientManager *ghclient.ClientManager
	clientGH      *ghclient.GHClient
	store         *badger.DB
	DB 				*gorm.DB
	cachePath     = "./shared/data/httpcache"
	storagePath   = "./shared/data/badger"
	debug         = false
	isSelenium    = false
	isTwitter     = false
	isFollow      = true
	isStar        = true
	isReadme      = true
	logLevelStr   = "info"
	maxTweetLen   = 280
	addMedia      = false
)

type VcsInfo struct {
	Desc string
	URL  string
	Lang string
	Tags []string
}

func InitDB() *gorm.DB {
	mysqlString := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=True&loc=Local&charset=utf8mb4,utf8", "root", "aado33ve79T!", "127.0.0.1", "3306", "gopress")

	//psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=disable", host, port, user, dbname, password)
	db, err := gorm.Open("mysql", mysqlString)
	if err != nil {
		panic(err)
	}
	db.LogMode(true)
	DB = db

	var post models.Post
	var video []models.Video
	var image []models.Image
	var link []models.Link
	var documents []models.Document

	TruncateTables(&models.Post{},  &models.Tag{}, &models.Category{})
	DB.Set("gorm:table_options", "CHARSET=utf8mb4").AutoMigrate(&models.Event{}, &models.Category{}, &models.Post{},  &models.Tag{}, &models.Document{}, &models.Video{}, &models.Image{}, &models.Link{})

	DB.Model(&post).Related(&video)
	DB.Model(&post).Related(&image)
	DB.Model(&post).Related(&link)
	DB.Model(&post).Related(&documents)
	return DB
}

func openFileByURL(rawURL string) (*os.File, error) {
	if fileURL, err := url.Parse(rawURL); err != nil {
		return nil, err
	} else {
		path := fileURL.Path
		segments := strings.Split(path, "/")
		fileName := segments[len(segments)-1]

		filePath := filepath.Join(os.TempDir(), fileName)

		if _, err := os.Stat(filePath); err == nil {
			return os.Open(filePath)
		}

		file, err := os.Create(filePath)
		if err != nil {
			return file, err
		}

		check := http.Client{
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				r.URL.Opaque = r.URL.Path
				return nil
			},
		}
		resp, err := check.Get(rawURL) // add a filter to check redirect
		if err != nil {
			return file, err
		}
		defer resp.Body.Close()
		fmt.Printf("----> Downloaded %v\n", rawURL)

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return file, err
		}
		return file, nil
	}
}

func TruncateTables(tables ...interface{}) {
	for _, table := range tables {
		if err := DB.DropTableIfExists(table).Error; err != nil {
			panic(err)
		}
		// DB.AutoMigrate(table)
	}
}

func GetDB() *gorm.DB {
	return DB
}

func createOrUpdatePost(db *gorm.DB, post *models.Post, tag *models.Tag) (bool, error) {
	var existingPost models.Post
	if db.Where("slug = ?", post.Slug).First(&existingPost).RecordNotFound() {
		err := db.Create(post).Error
		return err == nil, err
	}
	var existingTag models.Tag
	if db.Where("name = ?", tag.Name).First(&existingTag).RecordNotFound() {
		err := db.Create(tag).Error
		return err == nil, err
	}
	post.ID = existingPost.ID
	// post.CreatedAt = existingPost.CreatedAt
	post.Tags = append(post.Tags, existingTag)
	return false, db.Save(post).Error
}

func main() {

	DB = InitDB()
	m := cmap.New()

	// read .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	err = ensureDir(storagePath)
	if err != nil {
		log.Fatal(err)
	}
	store, err = badger.Open(badger.DefaultOptions(storagePath))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	// github client init
	clientManager = ghclient.NewManager(cachePath, []string{os.Getenv("GITHUB_TOKEN")})
	defer clientManager.Shutdown()
	clientGH = clientManager.Fetch()

	// Create a Collector specifically for Shopify
	c := colly.NewCollector(
		colly.UserAgent(uarand.GetRandom()),
		colly.AllowedDomains("www.kitploit.com"),
		colly.CacheDir("./data/cache"),
	)

	// create a request queue with 2 consumer threads
	q, _ := queue.New(
		20, // Number of consumer threads
		&queue.InMemoryQueueStorage{
			MaxSize: 100000,
		}, // Use default queue storage
	)

	c.OnRequest(func(r *colly.Request) {
		log.Println("visiting", r.URL)
	})

	// Create a callback on the XPath query searching for the URLs
	c.OnXML("//sitemap/loc", func(e *colly.XMLElement) {
		// knownUrls = append(knownUrls, e.Text)
		q.AddURL(e.Text)
	})

	// Create a callback on the XPath query searching for the URLs
	c.OnXML("//urlset/url/loc", func(e *colly.XMLElement) {
		// knownUrls = append(knownUrls, e.Text)
		q.AddURL(e.Text)
	})

	c.OnHTML("div.blog-posts.hfeed", func(e *colly.HTMLElement) {
		e.ForEach("a[href]", func(_ int, eli *colly.HTMLElement) {
			if strings.HasPrefix(eli.Attr("href"), "https://github.com") {
				var vcsUrl string
				if info, err := vcsurl.Parse(eli.Attr("href")); err == nil {
					vcsUrl = fmt.Sprintf("https://github.com/%s/%s", info.Username, info.Name)
					// githubUrls = append(githubUrls, vcsUrl)
					// githubUrls[vcsUrl] = true
					log.Println("found href=", vcsUrl)
				}
				var topics []string
				e.ForEach(".label-head > a", func(_ int, eli *colly.HTMLElement) {
					topic := strcase.ToCamel(fmt.Sprintf("%s", eli.Attr("title")))
					topic = strings.Replace(topic, " ", "", -1)
					topic = strings.Replace(topic, "!", "", -1)
					topic = strings.Replace(topic, "/", "", -1)
					topic = strings.Replace(topic, "'", "", -1)
					// log.Println("topic: ", fmt.Sprintf("#%s", topic))
					topics = append(topics, fmt.Sprintf("%s", topic))
				})
				if vcsUrl != "" {
					m.Set(vcsUrl, strings.Join(topics, ","))
				}
			}
		})
	})

	q.AddURL("https://www.kitploit.com/sitemap.xml?1")

	// Consume URLs
	q.Run(c)

	log.Println("All github URLs:")
	// log.Println("Collected", len(githubUrls), "URLs")
	log.Println("Collected cmap: ", m.Count(), "URLs")


	t := throttler.New(2, m.Count())

	// counter := 1
	// counterFollow := 1
	m.IterCb(func(key string, v interface{}) {
		var topics string
		_, ok := v.(string)
		if ok {
			topics = v.(string)
		}

		go func(key, topics string) error {
			// Let Throttler know when the goroutine completes
			// so it can dispatch another worker
			defer t.Done(nil)

			var imgLinks []string
			if info, err := vcsurl.Parse(key); err == nil {

				repoInfo, err := getInfo(clientGH.Client, info.Username, info.Name)
				if err != nil {
					log.Warnln(err)
					return err
				}

				readme, err := getReadme(clientGH.Client, info.Username, info.Name)
				if err != nil {
					log.Warnln(err)
					return err
				}
				// pp.Println(readme)
				imgPatternRegexp, err := regexp.Compile(`(http(s?):)([/|.|\w|\s|-])*\.(?:jpg|gif|GIF|jpeg|JPG|JPEG)`)
				// imgPatternRegexp, err := regexp.Compile(`(http(s?):)([/|.|\w|\s|-])*\.(?:gif|GIF)`)
				if err != nil {
					log.Warnln(err)
					return err
				}
				imgLinks = imgPatternRegexp.FindAllString(readme, -1)
				imgRelRegexp, err := regexp.Compile(`([/|.|\w|\s|-])*\.(?:jpg|gif|GIF|jpeg|JPG|JPEG)`)
				if err != nil {
					log.Warnln(err)
					return err
				}
				imgLinksRel := imgRelRegexp.FindAllString(readme, -1)
				for i, imgRel := range imgLinksRel {
					if strings.HasPrefix(imgRel, "//") {
						imgLinksRel[i] = "https:" + imgRel
					} else {
						imgLinksRel[i] = key + "/raw/master/" + imgRel
					}
				}
				imgLinks = append(imgLinks, imgLinksRel...)
				imgLinks = removeDuplicates(imgLinks)
				// pp.Println(imgLinksRel)
				pp.Println(imgLinks)
				/*
				// ensure dir
				prefixPath := filepath.Join("/Users/lucmichalski/go/src/github.com/x0rzkov/hugo-website/x0rzkov/content", "github.com", info.Username, info.Name)
				if err := ensureDir(prefixPath); err != nil {
					log.Fatal(err)
				}

				// write file
				fileFullPath := filepath.Join(prefixPath, "README.md")
				f, err := os.Create(fileFullPath)
				if err != nil {
					fmt.Println(err)
					return err
				}
				*/

				var title, desc string
				if repoInfo.Description != nil {
					desc = strings.TrimSpace(*repoInfo.Description)
					if len(desc) > 255 {
						title = desc[0:255]
					} else {
						title = desc
					}
					if title == "" {
						title = *repoInfo.Name
					}
				}

				var extTopics []string 
				extTopics = append(extTopics, repoInfo.Topics...)
				kitTopics := strings.Split(topics, ",")
				extTopics = append(extTopics, kitTopics...)
				extTopics = removeDuplicates(extTopics)

				pp.Println("extTopics: ", extTopics)

				// unsafe := blackfriday.Run([]byte(readme))
				// html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)

				// extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.Tables | parser.FencedCode | parser.Mmark
				parser := parser.NewWithExtensions(parser.CommonExtensions)

				if readme == "" {
					return nil
				}

				// md := []byte("## markdown document")
				html := markdown.ToHTML([]byte(readme), parser, nil)

				for _, extTopic := range extTopics {
					var images []models.Image
					for _, imgLink := range imgLinks {
						var image models.Image
						if file, err := openFileByURL(imgLink); err != nil {
							fmt.Printf("open file (%q) failure, got err %v", imgLink, err)
						} else {
							image.File.Scan(file)
							images = append(images, image)
							file.Close()
						}

						if err := DB.Create(&image).Error; err != nil {
							log.Fatalf("create image, got err %v when %v", err)
						}
					}

					p := &models.Post{
						Title: *repoInfo.Name,
						Slug: "github-"+info.Username+"-"+info.Name,
						Body: string(html),
						Summary: desc,
						Images: images,
					}
					c := &models.Tag{
						Name: extTopic,
					}

					if _, err := createOrUpdatePost(DB, p, c); err != nil {
						log.Warnln(err)
					}
				}
				return nil
			}
			return nil

		}(key, topics)

		t.Throttle()

	})

	// throttler errors iteration
	if t.Err() != nil {
		// Loop through the errors to see the details
		for i, err := range t.Errs() {
			log.Printf("error #%d: %s", i, err)
		}
		log.Fatal(t.Err())
	}

}

func addslashes(str string) string {
	var tmpRune []rune
	strRune := []rune(str)
	for _, ch := range strRune {
		switch ch {
		case []rune{'\\'}[0], []rune{'"'}[0] :
			tmpRune = append(tmpRune, []rune{'\\'}[0])
			tmpRune = append(tmpRune, ch)
		default:
			tmpRune = append(tmpRune, ch)
		}
	}
	return string(tmpRune)
}

func escape(sql string) string {
	dest := make([]byte, 0, 2*len(sql))
	var escape byte
	for i := 0; i < len(sql); i++ {
		c := sql[i]

		escape = 0

		switch c {
		case 0: /* Must be escaped for 'mysql' */
			escape = '0'
			break
		case '\n': /* Must be escaped for logs */
			escape = 'n'
			break
		case '\r':
			escape = 'r'
			break
		case '\\':
			escape = '\\'
			break
		case '\'':
			escape = '\''
			break
		case '"': /* Better safe than sorry */
			escape = '"'
			break
		case '\032': /* This gives problems on Win32 */
			escape = 'Z'
		}

		if escape != 0 {
			dest = append(dest, '\\', escape)
		} else {
			dest = append(dest, c)
		}
	}

	return string(dest)
}

// HTTP GET timeout
const TIMEOUT = 20

func downloadAsOne(url, out string) (int64, error) {
	var client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 30,
		},
		Timeout: TIMEOUT * time.Second,
	}

	resp, err := client.Get(url)

	if err != nil {
		log.Println("Trouble making GET photo request!")
		return 0, err
	}

	defer resp.Body.Close()

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Trouble reading response body!")
		return 0, err
	}

	err = ioutil.WriteFile(out, contents, 0644)
	if err != nil {
		log.Println("Trouble creating file!")
		return 0, err
	}

	fi, err := os.Stat(out)
	if err != nil {
		return 0, err
	}
	// get the size
	size := fi.Size()

	fmt.Printf("The file is %d bytes long", fi.Size())
	return size, nil
}

func removeDuplicates(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]bool{}
	result := []string{}

	for v := range elements {
		elements[v] = strings.ToLower(elements[v])
		if encountered[elements[v]] == true {
			// Do not add duplicate.
		} else {
			// Record this element as an encountered element.
			encountered[elements[v]] = true
			// Append to result slice.
			result = append(result, elements[v])
		}
	}
	// Return the new slice.
	return result
}

func canTweet(s string) bool {
	if utf8.RuneCountInString(s) > maxTweetLen {
		return false
	}
	return true
}

func getFromBadger(key string) (resp []byte, ok bool) {
	err := store.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			// This func with val would only be called if item.Value encounters no error.
			// Accessing val here is valid.
			// fmt.Printf("The answer is: %s\n", val)
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	return resp, err == nil
}

func addToBadger(key, value string) error {
	err := store.Update(func(txn *badger.Txn) error {
		if debug {
			log.Println("indexing: ", key)
		}
		cnt, err := compress([]byte(value))
		if err != nil {
			return err
		}
		err = txn.Set([]byte(key), cnt)
		return err
	})
	return err
}

func compress(data []byte) ([]byte, error) {
	return snappy.Encode([]byte{}, data), nil
}

func decompress(data []byte) ([]byte, error) {
	return snappy.Decode([]byte{}, data)
}

func ensureDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		os.MkdirAll(path, os.FileMode(0755))
	} else {
		return err
	}
	d.Close()
	return nil
}

func followUsername(client *github.Client, username string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitForRemainingLimit(client, true, 10)
	resp, err := client.Users.Follow(ctx, username)
	if err != nil {
		bs, _ := ioutil.ReadAll(resp.Body)
		log.Errorf("follow %s err: %s [%s]", username, bs, err)
		return false, err
	}
	return true, nil
}

func starRepo(client *github.Client, owner, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitForRemainingLimit(client, true, 10)
	resp, err := client.Activity.Star(ctx, owner, name)
	if err != nil {
		bs, _ := ioutil.ReadAll(resp.Body)
		log.Errorf("star %s/%s err: %s [%s]", owner, name, bs, err)
		return false, err
	}
	return true, nil
}

func getInfo(client *github.Client, owner, name string) (*github.Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitForRemainingLimit(client, true, 10)
	info, _, err := client.Repositories.Get(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func getTopics(client *github.Client, owner, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitForRemainingLimit(client, true, 10)
	topics, _, err := client.Repositories.ListAllTopics(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	return topics, nil
}

func getReadme(client *github.Client, owner, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitForRemainingLimit(client, true, 10)
	readme, _, err := client.Repositories.GetReadme(ctx, owner, name, nil)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	content, err := readme.GetContent()
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return content, nil
}

func waitForRemainingLimit(cl *github.Client, isCore bool, minLimit int) {
	for {
		rateLimits, _, err := cl.RateLimits(context.Background())
		if err != nil {
			if debug {
				log.Printf("could not access rate limit information: %s\n", err)
			}
			<-time.After(time.Second * 1)
			continue
		}

		var rate int
		var limit int
		if isCore {
			rate = rateLimits.GetCore().Remaining
			limit = rateLimits.GetCore().Limit
		} else {
			rate = rateLimits.GetSearch().Remaining
			limit = rateLimits.GetSearch().Limit
		}

		if rate < minLimit {
			if debug {
				log.Printf("Not enough rate limit: %d/%d/%d\n", rate, minLimit, limit)
			}
			<-time.After(time.Second * 60)
			continue
		}
		if debug {
			log.Printf("Rate limit: %d/%d\n", rate, limit)
		}
		break
	}
}

