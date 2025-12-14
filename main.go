package main

import (
	"log"
	"os"

	"codereviewAI/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Warn(".env file not found")
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	requiredEnvVars := []string{"GITLAB_TOKEN", "OPENAI_API_KEY", "GITLAB_PROJECT_ID"}
	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			logrus.Fatalf("Missing required environment variable: %s", envVar)
		}
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

	gitlabService := services.NewGitLabService()
	aiService := services.NewAIService()
	reviewService := services.NewReviewService(gitlabService, aiService)

	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "OK", "message": "Code Review AI is running correctly"})
		})

		api.GET("/merge-requests", reviewService.GetMergeRequests)
		api.GET("/merge-requests/:id", reviewService.GetMergeRequest)
		api.POST("/merge-requests/:id/review", reviewService.ReviewMergeRequest)
		api.GET("/projects", reviewService.GetProjects)
		api.GET("/stats", reviewService.GetStats)
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
