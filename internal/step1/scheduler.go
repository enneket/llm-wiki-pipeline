package step1

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler manages periodic feed fetching
type Scheduler struct {
	cron           *cron.Cron
	fetcher        *Fetcher
	feeds          []Feed
	globalInterval string
	rawDataPath    string
	onNewItem      func(feedName string, item *Item, filePath string)
	onProgress     func(total, completed int, current string)
}

func NewScheduler(fetcher *Fetcher, rawDataPath string) *Scheduler {
	return &Scheduler{
		cron:        cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))),
		fetcher:     fetcher,
		rawDataPath: rawDataPath,
	}
}

// SetGlobalInterval sets the default interval for all feeds
func (s *Scheduler) SetGlobalInterval(interval string) {
	s.globalInterval = interval
}

// Register adds a feed to the scheduler
func (s *Scheduler) Register(feed Feed) error {
	interval := feed.Interval
	if interval == "" {
		interval = s.globalInterval
	}
	_, err := s.cron.AddFunc(interval, func() {
		s.fetchOne(feed)
	})
	if err != nil {
		return fmt.Errorf("add cron func for %s: %w", feed.Name, err)
	}
	s.feeds = append(s.feeds, feed)
	return nil
}

// OnNewItem registers a callback for each new item fetched
func (s *Scheduler) OnNewItem(fn func(feedName string, item *Item, filePath string)) {
	s.onNewItem = fn
}

// OnProgress registers a callback for fetch progress updates
func (s *Scheduler) OnProgress(fn func(total, completed int, current string)) {
	s.onProgress = fn
}

// Feeds returns the registered feeds
func (s *Scheduler) Feeds() []Feed {
	return s.feeds
}

// Start begins all scheduled jobs
func (s *Scheduler) Start(ctx context.Context) {
	s.cron.Start()
	log.Printf("[scheduler] started with %d feeds", len(s.feeds))
	<-ctx.Done()
	s.cron.Stop()
}

func (s *Scheduler) fetchOne(feed Feed) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Exponential backoff retry: 3 attempts, max 5min total
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Minute
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			log.Printf("[scheduler] retrying %s in %v (attempt %d)", feed.Name, backoff, attempt+1)
			time.Sleep(backoff)
		}

		items, err := s.fetcher.Fetch(ctx, &feed)
		if err != nil {
			lastErr = err
			log.Printf("[scheduler] fetch %s failed: %v", feed.Name, err)
			continue
		}

		for _, item := range items {
			filePath, err := SaveToFile(&item, feed.Name, s.rawDataPath)
			if err != nil {
				log.Printf("[scheduler] save %s/%s failed: %v", feed.Name, item.Title, err)
				continue
			}
			if s.onNewItem != nil {
				s.onNewItem(feed.Name, &item, filePath)
			}
		}
		log.Printf("[scheduler] fetched %s: %d items", feed.Name, len(items))
		return
	}
	log.Printf("[scheduler] %s exhausted retries: %v", feed.Name, lastErr)
}

// RunOnce triggers all feeds once (for manual fetch or initial bootstrap)
func (s *Scheduler) RunOnce() {
	total := len(s.feeds)
	completed := 0

	if s.onProgress != nil {
		s.onProgress(total, 0, "")
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, feed := range s.feeds {
		wg.Add(1)
		go func(f Feed) {
			defer wg.Done()
			if s.onProgress != nil {
				s.onProgress(total, completed, f.Name)
			}
			s.fetchOne(f)
			mu.Lock()
			completed++
			mu.Unlock()
			if s.onProgress != nil {
				s.onProgress(total, completed, f.Name)
			}
		}(feed)
	}
	wg.Wait()
}
