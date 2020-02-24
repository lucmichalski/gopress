package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/Masterminds/sprig"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"github.com/koreset/gtf"
	"github.com/qor/admin"
	"github.com/qor/media"
	"github.com/qor/media/asset_manager"
	"github.com/qor/media/media_library"
	"github.com/qor/qor"
	uuid "github.com/satori/go.uuid"
	stats "github.com/semihalev/gin-stats"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/swaggo/gin-swagger/example/basic/docs"

	"github.com/lucmichalski/gopress/pkg/config/bindatafs"
	"github.com/lucmichalski/gopress/pkg/controllers"
	"github.com/lucmichalski/gopress/pkg/models"
	"github.com/lucmichalski/gopress/pkg/services"
	"github.com/lucmichalski/gopress/pkg/utils"
)

var db *gorm.DB
var funcMaps template.FuncMap
var templates *template.Template

// AutoMigrate run auto migration
func AutoMigrate(values ...interface{}) {
	for _, value := range values {
		db.AutoMigrate(value)
	}
}

func SetupDB() {
	db = services.Init()
	db.LogMode(true)
	db.AutoMigrate(
		// generic
		&models.Post{}, 
		&models.Video{}, 
		&models.Image{}, 
		&models.Link{}, 
		&models.Category{},
		// custom to oniontree
		&models.Service{}, 
		&models.URL{}, 
		&models.PublicKey{},		
	)
	media.RegisterCallbacks(db)
}

func setupTemplateFuncs() template.FuncMap {
	funcMaps := sprig.FuncMap()
	funcMaps["unsafeHtml"] = utils.UnsafeHtml
	funcMaps["stripSummaryTags"] = utils.StripSummaryTags
	funcMaps["displayDateString"] = utils.DisplayDateString
	funcMaps["displayDate"] = utils.DisplayDateV2
	funcMaps["truncateBody"] = utils.TruncateBody

	gtf.Inject(funcMaps)
	return funcMaps
}

func RequestIdMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		xuid, _ := uuid.NewV4()
		c.Writer.Header().Set("X-Request-Id", xuid.String())
		c.Next()
	}
}

func SetupRouter() *gin.Engine {
	mux := http.NewServeMux()

	Admin := admin.New(&admin.AdminConfig{DB: db})
	Admin.SetAssetFS(bindatafs.AssetFS.NameSpace("admin"))

	Admin.MountTo("/admin", mux)

	//API Setup
	API := admin.New(&qor.Config{DB: db})
	API.AddResource(&models.Post{})

	API.MountTo("/adminapi", mux)

	assetManager := Admin.AddResource(&asset_manager.AssetManager{}, &admin.Config{Invisible: true})

	Admin.AddResource(&media_library.MediaLibrary{}, &admin.Config{Menu: []string{"Site Management"}})
	Admin.AddResource(&models.Category{}, &admin.Config{
		Name: "Categories",
		Menu: []string{"Content Management"},
	})

	Admin.AddResource(&models.Tag{}, &admin.Config{
		Name: "Tags",
		Menu: []string{"Content Management"},
	})

	post := Admin.AddResource(&models.Post{}, &admin.Config{
		Name: "Posts",
		Menu: []string{"Content Management"},
	})

	post.IndexAttrs("ID", "Title", "Summary", "Type", "Categories")
	post.NewAttrs("Title", "Summary", "Images", "Videos", "Links", "Documents", "Categories", "Type")
	post.Meta(&admin.Meta{
		Name: "Body",
		Config: &admin.RichEditorConfig{
			AssetManager: assetManager,
		},
	})
	post.Meta(&admin.Meta{
		Name: "Type",
		Type: "select_one",
		Config: &admin.SelectOneConfig{
			Collection: []string{"article", "publication", "blog", "video", "press_release", "event", "news"},
		},
	})

	// custom section for oniontree
	Admin.AddResource(&models.Service{}, &admin.Config{
		Name: "Service",
		Menu: []string{"Oniontree Management"},
	})

	Admin.AddResource(&models.URL{}, &admin.Config{
		Name: "URL",
		Menu: []string{"Oniontree Management"},
	})

	Admin.AddResource(&models.PublicKey{}, &admin.Config{
		Name: "PublicKey",
		Menu: []string{"Oniontree Management"},
	})

	router := gin.Default()

	if runtime.GOOS == "linux" {
		log.Println("Loading html from binary")
		router.SetHTMLTemplate(templates)
	}

	if runtime.GOOS == "darwin" {
		router.SetFuncMap(setupTemplateFuncs())
		router.LoadHTMLGlob("views/**/*")
	}

	router.GET("/stats", func(context *gin.Context) {
		context.JSON(http.StatusOK, stats.Report())
	})
	router.GET("/", controllers.Home)
	router.GET("/about-us", controllers.AboutUs)
	router.GET("/categories/:category", controllers.GetPostsForCategory)
	//router.GET("/resources", controllers.ResourceIndex)
	// router.GET("/resources/publications", controllers.ResourcePublications)
	// router.GET("/resources/publications/:page", controllers.ResourcePublications)
	// router.GET("/resources/books", controllers.ResourceBooks)
	//router.GET("/resources/gallery", controllers.ResourceGallery)
	// router.GET("/resources/gallery/:albumid/:albumtitle", controllers.ResourceGalleryDetail)
	router.GET("posts/:slug", controllers.GetPost)
	// router.GET("publications", controllers.GetPublications)
	router.GET("/test", controllers.GetTest)
	router.GET("/news", controllers.GetNews)
	router.GET("/news/:page", controllers.GetNews)
	router.GET("/new", controllers.GetNew)
	router.GET("/boot", controllers.GetBoot)

	// router.Static("/public", "./public")
	// router.Static("/assets", "./assets")

	url := ginSwagger.URL("http://localhost:4000/swagger/doc.json") // The url pointing to API definition
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, url))

	//API Calls
	api := router.Group("/api")
	{
		api.GET("/get-tweets", controllers.GetTweets)
		api.GET("/get-flickr", controllers.GetFlickr)
		api.GET("/testdata", controllers.GetTestData)
	}

	router.Static("/public", "./public")

	admin := router.Group("/admin", gin.BasicAuth(gin.Accounts{
		"admin":  "admin",
		"jome":   "wordpass15",
		"nnimmo": "wordpass15"}))
	{
		admin.Any("/*resources", gin.WrapH(mux))
	}
	router.Any("/adminapi/*resources", gin.WrapH(mux))
	router.NoRoute(func(context *gin.Context) {
		fmt.Println(">>>>>>>>>>>>>>>>>> 404 <<<<<<<<<<<<<<<<<<<")
		context.HTML(http.StatusNotFound, "content_not_found", nil)
	})
	return router
}

func loadTemplates() (*template.Template, error) {
	templates = template.New("")
	templates.Funcs(setupTemplateFuncs())
	var myAssets = Assets.Files

	for name, file := range myAssets {
		if file.IsDir() || !strings.HasSuffix(name, ".html") {
			continue
		}
		h, err := ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}
		templates, err = templates.New(name).Parse(string(h))
		if err != nil {
			return nil, err
		}
	}

	return templates, nil
}

func main() {
	port := flag.String("port", "4000", "The port the app will listen to")
	host := flag.String("host", "0.0.0.0", "The ip address to listen on")
	compileTemplate := flag.Bool("compile-templates", false, "Set this to true to compile templates to binary")

	flag.Parse()

	if *compileTemplate {
		Admin := admin.New(&admin.AdminConfig{
			DB:      db,
			AssetFS: bindatafs.AssetFS.NameSpace("admin")})
		Admin.SetAssetFS(bindatafs.AssetFS.NameSpace("admin"))
		bindatafs.AssetFS.Compile()
	} else {
		SetupDB()
		defer db.Close()

		loadTemplates()
		r := SetupRouter()
		fmt.Println(*host, *port)

		//if runtime.GOOS == "linux"{
		//	log.Fatal(autotls.Run(r, "beta.homef.org", "homef.org", "www.homef.org"))
		//}else{
		//	r.Run(fmt.Sprintf("%s:%s", *host, *port))
		//}

		r.Run(fmt.Sprintf("%s:%s", *host, *port))
	}
}
