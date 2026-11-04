package main

import (
	"log"
	"os"

	"codereviewAI/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

type rs = services.ReviewService

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Warn(".env file not found")
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// SQLite: settings changed in the UI (they override .env) and review results
	store, err := services.OpenStore(services.DBPath())
	if err != nil {
		logrus.Fatalf("Cannot open database: %v", err)
	}
	defer store.Close()
	logrus.Infof("Database: %s", store.Path())

	settings, err := services.NewSettingsManager(store)
	if err != nil {
		logrus.Fatalf("Cannot load settings: %v", err)
	}
	if os.Getenv("GITLAB_TOKEN") == "" {
		logrus.Warn("GITLAB_TOKEN is not set - open the web interface and configure it in Settings")
	}

	wsHub := services.NewWSHub()
	go wsHub.Run()

	rt, err := services.NewRuntime(wsHub, store, settings)
	if err != nil {
		logrus.Fatalf("Cannot initialise services: %v", err)
	}

	router := gin.Default()

	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	router.Static("/static", "./static")

	router.GET("/", func(c *gin.Context) {
		c.File("static/index.html")
	})

	router.GET("/api/v1/ws", func(c *gin.Context) {
		wsHub.HandleWS(c)
	})

	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "OK", "message": "Code Review AI is running correctly"})
		})

		api.GET("/config", rt.Handle((*rs).GetConfig))
		api.GET("/projects", rt.Handle((*rs).GetProjects))
		api.GET("/projects/:pid/merge-requests", rt.Handle((*rs).GetMergeRequests))
		api.GET("/projects/:pid/merge-requests/:id", rt.Handle((*rs).GetMergeRequest))
		api.POST("/projects/:pid/merge-requests/:id/review", rt.Handle((*rs).ReviewMergeRequest))
		api.GET("/projects/:pid/merge-requests/:id/reviews", rt.Handle((*rs).ReviewHistory))
		api.DELETE("/projects/:pid/merge-requests/:id/reviews", rt.Handle((*rs).DeleteReviews))

		api.GET("/reviews", rt.Handle((*rs).ListReviews))
		api.POST("/reviews/import", rt.Handle((*rs).ImportReviews))

		api.GET("/settings", rt.GetSettings)
		api.PUT("/settings", rt.UpdateSettings)
		api.POST("/settings/test/:target", rt.TestSettings)

		// legacy routes - default project (GITLAB_PROJECT_ID)
		api.GET("/merge-requests", rt.Handle((*rs).GetMergeRequests))
		api.GET("/merge-requests/:id", rt.Handle((*rs).GetMergeRequest))
		api.POST("/merge-requests/:id/review", rt.Handle((*rs).ReviewMergeRequest))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logrus.Infof("Server starting on port %s", port)
	logrus.Infof("Web interface available at: http://localhost:%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Error starting server:", err)
	}
}
