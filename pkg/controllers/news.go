package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lucmichalski/gopress/pkg/services"
	"github.com/lucmichalski/gopress/pkg/utils"
)

func GetNews(c *gin.Context) {

	page, err := strconv.Atoi(c.Param("page"))
	if err != nil {
		page = 0
	}

	payload := make(map[string]interface{})
	newsItems := services.GetPosts(utils.NewsLimit*page, 10)
	recentPosts := services.GetPosts(0, 5)
	payload["recentPosts"] = recentPosts
	payload["newsitems"] = newsItems
	payload["title"] = "News"
	payload["nextPage"] = page + 1

	c.HTML(http.StatusOK, "news_index", payload)
}
