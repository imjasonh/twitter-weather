package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	language "cloud.google.com/go/language/apiv1"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"golang.org/x/net/context"
	lpb "google.golang.org/genproto/googleapis/cloud/language/v1"
)

var (
	consumerKey    = flag.String("consumer-key", "", "Twitter Consumer Key")
	consumerSecret = flag.String("consumer-secret", "", "Twitter Consumer Secret")
	accessToken    = flag.String("access-token", "", "Twitter Access Token")
	accessSecret   = flag.String("access-secret", "", "Twitter Access Secret")
)

func main() {
	flag.Parse()

	// Twitter Client
	client := twitter.NewClient(
		oauth1.NewConfig(*consumerKey, *consumerSecret).Client(oauth1.NoContext,
			oauth1.NewToken(*accessToken, *accessSecret)))

	// Google NLP Client
	ctx := context.Background()
	c, err := language.NewClient(ctx)
	if err != nil {
		log.Fatalf("Setting up Cloud client: %v", err)
	}
	a := analyzer{c}

	stream, err := client.Streams.Sample(&twitter.StreamSampleParams{
		Language:      []string{"en"}, // Only English.
		StallWarnings: twitter.Bool(true),
	})
	if err != nil {
		log.Fatalf("Stream.Sample: %v", err)
	}
	demux := twitter.NewSwitchDemux()
	demux.Tweet = a.Analyze
	demux.Warning = func(warning *twitter.StallWarning) {
		fmt.Println("WARNING:", warning.Message)
	}

	fmt.Println("Starting Stream...")
	go demux.HandleChan(stream.Messages)

	// Wait for SIGINT and SIGTERM (HIT CTRL-C)
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	fmt.Println("Stopping Stream...")
	stream.Stop()
}

type analyzer struct {
	client *language.Client
}

func (a analyzer) Analyze(t *twitter.Tweet) {
	ctx := context.Background()
	resp, err := a.client.AnalyzeSentiment(ctx, &lpb.AnalyzeSentimentRequest{
		Document: &lpb.Document{
			Type:   lpb.Document_PLAIN_TEXT,
			Source: &lpb.Document_Content{t.Text},
		},
	})
	if err != nil {
		log.Println("Analyze error:", err)
		return
	}
	log.Println("Sentiment:", resp.DocumentSentiment.Score)
}
