// Command guardpipe is the entrypoint for the GuardPipe API/worker binary.
//
// This is a Phase 0 scaffold: it proves the binary builds, runs in a
// container, and answers health checks. Typed config, structured logging,
// and the real router live in internal/platform and internal/transport
// (Phase 1+) and will replace the inline logic here.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	version   = "dev"
	commitSHA = "unknown"
	buildTime = "unknown"
)

func main() {
	// The distroless runtime image has no shell, curl, or wget, so a Docker
	// Compose HEALTHCHECK cannot exec a shell command inside the container.
	// It execs the binary itself instead: "guardpipe healthcheck".
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	port := os.Getenv("GUARDPIPE_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	role := os.Getenv("GUARDPIPE_ROLE")
	if role == "" {
		role = "all"
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/readyz", func(c *gin.Context) {
		// Phase 1 wires real Postgres/Redis checks here.
		c.Status(http.StatusOK)
	})
	router.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version": version,
			"commit":  commitSHA,
			"built":   buildTime,
		})
	})

	log.Printf("guardpipe starting: role=%s port=%s version=%s", role, port, version)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func runHealthcheck() int {
	port := os.Getenv("GUARDPIPE_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:" + port + "/readyz")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
