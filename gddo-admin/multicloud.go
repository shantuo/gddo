package main

import (
	"context"
	"encoding/csv"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/gddo/database"
	"github.com/golang/gddo/httputil"
	"github.com/google/go-github/github"
)

var multicloudCommand = &command{
	name:  "multicloud",
	run:   multicloud,
	usage: "cross check packages running on multi clouds",
}

func multicloud(c *command) {
	depGCP := []string{
		"cloud.google.com/go/storage",
		"google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1",
		"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/certs",
		"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/dialers/mysql",
		"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy",
		"google.golang.org/api/cloudkms/v1",
		"cloud.google.com/go/pubsub",
	}

	depAWS := []string{
		"github.com/aws/aws-sdk-go/aws",
		"github.com/aws/aws-sdk-go/aws/client",
		"github.com/aws/aws-sdk-go/aws/credentials",
		"github.com/aws/aws-sdk-go/aws/session",
		"github.com/aws/aws-sdk-go/aws/awserr",
		"github.com/aws/aws-sdk-go/service/s3",
		"github.com/aws/aws-sdk-go/service/s3/s3manager",
		"github.com/aws/aws-sdk-go/service/ssm",
		"github.com/aws/aws-sdk-go/service/sqs",
		"github.com/aws/aws-sdk-go/service/sns",
		"github.com/aws/aws-sdk-go/service/kms",
	}
	importers := make(map[string]int)
	db, err := database.New(*redisServer, *dbIdleTimeout, false, gaeEndpoint)
	if err != nil {
		log.Fatal(err)
	}

	for _, p := range depGCP {
		pkg, err := db.Importers(p)
		if err != nil {
			log.Println(err)
			continue
		}
		for _, t := range pkg {
			// For simplicity, we focus on github repos so the format is like
			// github.com/owner/repo.
			repo := strings.Join(strings.SplitN(t.Path, "/", 4)[:3], "/")
			importers[repo] |= 0x01
		}
	}
	for _, p := range depAWS {
		pkg, err := db.Importers(p)
		if err != nil {
			log.Println(err)
			continue
		}
		for _, t := range pkg {
			repo := strings.Join(strings.SplitN(t.Path, "/", 4)[:3], "/")
			importers[repo] |= 0x02
		}
	}

	gc := newGHClient()
	ctx := context.Background()
	pat := regexp.MustCompile(`^github\.com/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)$`)
	f, err := os.Create("multicloud.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"Name", "Stars", "Forks"})

	for k, v := range importers {
		if v == 3 {
			s := pat.FindStringSubmatch(k)
			if s == nil {
				w.Write([]string{k})
				continue
			}
			repo, _, err := gc.Repositories.Get(ctx, s[1], s[2])
			if err != nil {
				log.Println(err)
				continue
			}
			if !repo.GetFork() {
				w.Write([]string{k, strconv.Itoa(repo.GetStargazersCount()), strconv.Itoa(repo.GetForksCount())})
			}
		}
	}
}

func newGHClient() *github.Client {
	to := 30 * time.Second
	t := &httputil.AuthTransport{
		Base: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: to / 2,
			TLSHandshakeTimeout:   to / 2,
		},

		GithubToken:        os.Getenv("GITHUB_TOKEN"),
		GithubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GithubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
	}
	return github.NewClient(&http.Client{
		Transport: t,
		Timeout:   to,
	})
}
