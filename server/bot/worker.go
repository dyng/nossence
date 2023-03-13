package bot

import (
	"context"
	"time"

	n "github.com/dyng/nosdaily/nostr"
	"github.com/dyng/nosdaily/service"
	"github.com/ethereum/go-ethereum/log"
)

type Worker struct {
	client  n.ClientImpl
	service service.ServiceImpl
}

func NewWorker(ctx context.Context, client n.ClientImpl, service service.ServiceImpl) (*Worker, error) {
	return &Worker{
		client:  client,
		service: service,
	}, nil
}

func (w *Worker) Run(ctx context.Context, userPub, subSK string, timeRange time.Duration, limit int) error {
	_ = timeRange
	_ = limit
	feed := w.service.GetFeed() // TODO: should be GetFeed(userPub, timeRange, limit)
	if feed == nil {
		log.Warn("got empty feed", "userPub", userPub)
		return nil
	}

	if posts, ok := feed.([]service.FeedEntry); ok {
		for _, post := range posts {
			err := w.client.Repost(ctx, subSK, post.Id, post.Pubkey)
			if err != nil {
				log.Warn("failed to repost event", "id", post.Id, "err", err)
			}
		}
	}

	log.Info("reposted feed", "userPub", userPub)
	return nil
}
