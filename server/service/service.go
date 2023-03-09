package service

import (
	"context"
	"time"

	"github.com/dyng/nosdaily/database"
	"github.com/dyng/nosdaily/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/nbd-wtf/go-nostr"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Service struct {
	config *types.Config
	neo4j  *database.Neo4jDb
}

func NewService(config *types.Config, neo4j *database.Neo4jDb) *Service {
	return &Service{
		config: config,
		neo4j:  neo4j,
	}
}

func (s *Service) InitDatabase() error {
	_, err := s.neo4j.ExecuteWrite(func(tx neo4j.ManagedTransaction) (any, error) {
		ctx := context.Background()
		if _, err := tx.Run(ctx, "CREATE CONSTRAINT IF NOT EXISTS post_id_uniq FOR (p:Post) REQUIRE p.id IS UNIQUE;", nil); err != nil {
			return nil, err
		}
		if _, err := tx.Run(ctx, "CREATE CONSTRAINT IF NOT EXISTS user_pk_uniq FOR (u:User) REQUIRE u.pubkey IS UNIQUE;", nil); err != nil {
			return nil, err
		}
		return nil, nil
	})

	return err
}

func (s *Service) GetFeed() any {
	type feedEntry struct {
		Id        string    `json:"event_id"`
		Kind      int       `json:"kind"`
		Pubkey    string    `json:"pubkey"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		Summary   string    `json:"summary"`
		Title     string    `json:"title"`
		Image     string    `json:"image"`
		Like      int       `json:"like"`
		Repost    int       `json:"repost"`
		Reply     int       `json:"reply"`
		Zap       int       `json:"zap"`
		Relay     []string  `json:"relay"`
	}

	posts, err := s.neo4j.ExecuteRead(func(tx neo4j.ManagedTransaction) (any, error) {
		ctx := context.Background()

		result, err := tx.Run(ctx, "match (p:Post) optional match (r:Post)-[:REPLY]->(p) with p, count(r) as replyCnt order by replyCnt desc limit 20 return p.id, p.kind, p.author, p.content, p.created_at, replyCnt;", nil)
		if err != nil {
			return nil, err
		}

		posts := make([]feedEntry, 0)
		for result.Next(ctx) {
			record := result.Record()
			post := feedEntry{
				Id:        record.Values[0].(string),
				Kind:      int(record.Values[1].(int64)),
				Pubkey:    record.Values[2].(string),
				Content:   record.Values[3].(string),
				CreatedAt: time.Unix(record.Values[4].(int64), 0),
				Reply:     int(record.Values[5].(int64)),
			}
			posts = append(posts, post)
		}
		return posts, nil
	})

	if err != nil {
		log.Error("Failed to get feed", "err", err)
		return nil
	} else {
		return posts
	}
}

func (s *Service) StoreEvent(event *nostr.Event) error {
	switch event.Kind {
	case 1, 30023:
		return s.StorePost(event)
	case 7:
		return s.StoreLike(event)
	default:
		log.Warn("Unsupported event kind", "kind", event.Kind)
		return nil
	}
}

func (s *Service) StorePost(event *nostr.Event) error {
	_, err := s.neo4j.ExecuteWrite(func(tx neo4j.ManagedTransaction) (any, error) {
		ctx := context.Background()

		// create user & post
		if err := s.saveUserAndPost(ctx, tx, event); err != nil {
			return nil, err
		}

		// create reply relation
		refs := event.Tags.GetAll([]string{"e"})
		if len(refs) > 0 {
			ref := refs[0]
			if _, err := tx.Run(ctx, "match (p:Post), (r:Post) where p.id = $Id and r.id = $RefId create (p)-[:REPLY]->(r);",
				map[string]any{
					"Id":    event.ID,
					"RefId": ref.Value(),
				}); err != nil {
				return nil, err
			}
		}

		return nil, nil
	})

	return err
}

func (s *Service) StoreLike(event *nostr.Event) error {
	_, err := s.neo4j.ExecuteWrite(func(tx neo4j.ManagedTransaction) (any, error) {
		ctx := context.Background()

		// create user & post
		if err := s.saveUserAndPost(ctx, tx, event); err != nil {
			return nil, err
		}

		// create like relation
		refs := event.Tags.GetAll([]string{"e"})
		if len(refs) > 0 {
			ref := refs[0]
			if _, err := tx.Run(ctx, "match (p:Post), (r:Post) where p.id = $Id and r.id = $RefId create (p)-[:LIKE]->(r);",
				map[string]any{
					"Id":    event.ID,
					"RefId": ref.Value(),
				}); err != nil {
				return nil, err
			}
		}

		return nil, nil
	})

	return err
}

func (s *Service) saveUserAndPost(ctx context.Context, tx neo4j.ManagedTransaction, event *nostr.Event) error {
	if _, err := tx.Run(ctx, "merge (u:User {pubkey: $Pubkey});",
		map[string]any{
			"Pubkey": event.PubKey,
		}); err != nil {
		return err
	}

	if _, err := tx.Run(ctx, "merge (p:Post {id: $Id, kind: $Kind, author: $Author, content: $Content, created_at: $CreatedAt});",
		map[string]any{
			"Id":        event.ID,
			"Kind":      event.Kind,
			"Author":    event.PubKey,
			"Content":   event.Content,
			"CreatedAt": event.CreatedAt.Unix(),
		}); err != nil {
		return err
	}

	if _, err := tx.Run(ctx, "match (u:User), (p:Post) where u.pubkey = $Pubkey and p.id = $Id create (u)-[:CREATE]->(p);",
		map[string]any{
			"Pubkey": event.PubKey,
			"Id":     event.ID,
		}); err != nil {
		return err
	}

	return nil
}
