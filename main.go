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
		logrus.Warn("Nie znaleziono pliku .env")
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	requiredEnvVars := []string{"GITLAB_TOKEN", "OPENAI_API_KEY", "GITLAB_PROJECT_ID"}
	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			logrus.Fatalf("Brak wymaganej zmiennej środowiskowej: %s", envVar)
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
	router.LoadHTMLGlob("static/*.html")

	router.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	gitlabService := services.NewGitLabService()
	aiService := services.NewAIService()
	reviewService := services.NewReviewService(gitlabService, aiService)

	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "OK", "message": "Code Review AI działa poprawnie"})
		})

		api.GET("/merge-requests", reviewService.GetMergeRequests)
		api.GET("/merge-requests/:id", reviewService.GetMergeRequest)
		api.POST("/merge-requests/:id/review", reviewService.ReviewMergeRequest)
		api.GET("/projects", reviewService.GetProjects)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logrus.Infof("Serwer uruchamiany na porcie %s", port)
	logrus.Infof("Interfejs webowy dostępny pod adresem: http://localhost:%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Błąd uruchamiania serwera:", err)
	}
}
