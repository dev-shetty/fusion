package pull

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/repo"
)

type FeedRepo interface {
	All() ([]*model.Feed, error)
	Get(id uint) (*model.Feed, error)
	Update(id uint, feed *model.Feed) error
}

type ItemRepo interface {
	Creates(items []*model.Item) error
	IdentityExist(feedID uint, guid, link, title string) (bool, error)
}

type Puller struct {
	feedRepo FeedRepo
	itemRepo ItemRepo
}

func NewPuller(feedRepo FeedRepo, itemRepo ItemRepo) *Puller {
	return &Puller{
		feedRepo: feedRepo,
		itemRepo: itemRepo,
	}
}

const interval = 30

func (p *Puller) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticker := time.NewTicker(interval * time.Minute)
	defer ticker.Stop()

	for {
		p.PullAll(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Puller) PullAll(ctx context.Context) error {
	log.Println("start pull-all")
	ctx, cancel := context.WithTimeout(ctx, (interval-3)*time.Minute)
	defer cancel()
	feeds, err := p.feedRepo.All()
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			log.Println(err)
		}
		return err
	}
	if len(feeds) == 0 {
		return nil
	}

	routinePool := make(chan struct{}, 10)
	defer close(routinePool)
	wg := sync.WaitGroup{}
	for _, f := range feeds {
		if f.IsSuspended() || f.IsFailed() {
			log.Printf("skip %d\n", f.ID)
			continue
		}

		routinePool <- struct{}{}
		wg.Add(1)
		go func(f *model.Feed) {
			defer func() {
				wg.Done()
				<-routinePool
			}()

			if err := p.do(ctx, f); err != nil {
				log.Println(err)
			}
		}(f)
	}
	wg.Wait()
	return nil
}

func (p *Puller) PullOne(id uint) error {
	f, err := p.feedRepo.Get(id)
	if err != nil {
		log.Println(err)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return p.do(ctx, f)
}
