package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/compute/metadata"
	language "cloud.google.com/go/language/apiv1"
	"cloud.google.com/go/storage"
	"github.com/ImJasonH/go-twitter/twitter" // TODO: dghubble/go-twitter after https://github.com/dghubble/go-twitter/pull/46
	"github.com/dghubble/oauth1"
	"golang.org/x/net/context"
	lpb "google.golang.org/genproto/googleapis/cloud/language/v1"
)

var (
	// Twitter flags.
	consumerKey    = flag.String("consumer-key", "", "Twitter Consumer Key")
	consumerSecret = flag.String("consumer-secret", "", "Twitter Consumer Secret")
	accessToken    = flag.String("access-token", "", "Twitter Access Token")
	accessSecret   = flag.String("access-secret", "", "Twitter Access Secret")

	// Google flags.
	project      = flag.String("project", "", "GCP project ID")
	analyzeEvery = flag.Duration("analyze-every", 20*time.Second, "Frequency to send requests to NLP API")
	bucket       = flag.String("bucket", "", "GCS bucket to write output to")
	object       = flag.String("object", "", "GCS object to write output to")
	dataset      = flag.String("dataset", "", "BigQuery dataset to write output to")
	table        = flag.String("table", "", "BigQuery table to write output to")
	bqCacheSize  = flag.Int("bq_cache_size", 100, "Number of data points to cache before writing")
)

func flagFromMetadata(k string) {
	mdv, err := metadata.InstanceAttributeValue(k)
	if mdv != "" && err == nil {
		flag.Set(k, mdv)
	}
}

func main() {
	flag.Parse()

	flag.CommandLine.Visit(func(f *flag.Flag) {
		if metadata.OnGCE() {
		flagFromMetadata(f.Name)
	}
		log.Println(f.Name, f.Value.String())
	})

	// TODO: Recover from crashes by reading lastHour from data in GCS.

	// Twitter Client
	client := twitter.NewClient(
		oauth1.NewConfig(*consumerKey, *consumerSecret).Client(oauth1.NoContext,
			oauth1.NewToken(*accessToken, *accessSecret)))

	// Google NLP Client
	ctx := context.Background()
	l, err := language.NewClient(ctx)
	if err != nil {
		log.Fatalf("Setting up Language client: %v", err)
	}
	// Google Storage Client
	s, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Setting up GCS client: %v", err)
	}
	bq, err := bigquery.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("Setting up BigQuery client: %v", err)
	}
	a := &analyzer{l: l, s: storer{s}, bq: bigquerier{bq: bq}}

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
	l          *language.Client
	s          storer
	bq         bigquerier
	nextUpdate time.Time
	lastHour   []float32
}

func (a *analyzer) Analyze(t *twitter.Tweet) {
	// Don't analyze this tweet, too soon.
	if time.Now().Before(a.nextUpdate) {
		return
	}

	ctx := context.Background()
	resp, err := a.l.AnalyzeSentiment(ctx, &lpb.AnalyzeSentimentRequest{
		Document: &lpb.Document{
			Type:   lpb.Document_PLAIN_TEXT,
			Source: &lpb.Document_Content{t.Text},
		},
	})
	if err != nil {
		log.Println("Analyze error:", err)
		return
	}
	score := resp.DocumentSentiment.Score
	log.Println("Sentiment:", score)

	// Maybe write to BigQuery.
	a.bq.maybeUpload(score)

	// Write lastHour data to GCS.
	a.lastHour = append(a.lastHour, score)
	// Only keep last hour worth of data, so trim the oldest item.
	// For sample rate of 20s, lastHour will be 180 points long.
	if len(a.lastHour) > int(time.Hour / *analyzeEvery) {
		a.lastHour = a.lastHour[1:]
	}
	a.nextUpdate = time.Now().Add(*analyzeEvery)

	go a.s.update(a.lastHour)
}

type row struct {
	score     float32
	timestamp time.Time
}

func (r row) Save() (map[string]bigquery.Value, string, error) {
	return map[string]bigquery.Value{
		"score":     r.score,
		"timestamp": r.timestamp,
	}, "", nil
}

type bigquerier struct {
	cache []row
	bq    *bigquery.Client
}

func (b *bigquerier) maybeUpload(i float32) {
	ctx := context.Background()
	b.cache = append(b.cache, row{i, time.Now()})
	if len(b.cache) >= *bqCacheSize {
		u := b.bq.Dataset(*dataset).Table(*table).Uploader()
		if err := u.Put(ctx, b.cache); err != nil {
			log.Printf("Error uploading to BigQuery: %v", err)
			return
		}
		log.Println("Uploaded to BigQuery")
		b.cache = nil
	}
}

type storer struct {
	s *storage.Client
}

type gcsData struct {
	Timestamp time.Time `json:"timestamp"`
	Data      []float32 `json:"data"`
}

// update updates the object in GCS with latest data.
// TODO: Write the object with content/type and cache-control headers.
func (s storer) update(data []float32) {
	ctx := context.Background()
	o := s.s.Bucket(*bucket).Object(*object)
	w := o.NewWriter(ctx)
	if err := json.NewEncoder(w).Encode(gcsData{time.Now(), data}); err != nil {
		log.Println("JSON error: %v", err)
		return
	}
	if err := w.Close(); err != nil {
		log.Println("GCS error: %v", err)
		return
	}

	if _, err := o.Update(ctx, storage.ObjectAttrsToUpdate{
		ContentType:  "application/json",
		CacheControl: "max-age=59", // Cache for <60s.
	}); err != nil {
		log.Println("GCS update error: %v", err)
		return
	}

	log.Println("Updated GCS")
}
