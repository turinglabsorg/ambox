package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Store struct {
	agents *mongo.Collection
	emails *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{
		agents: db.Collection("agents"),
		emails: db.Collection("emails"),
	}
}

func (s *Store) EnsureIndexes(ctx context.Context) error {
	agentIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "api_key_prefix", Value: 1}}},
	}
	if _, err := s.agents.Indexes().CreateMany(ctx, agentIndexes); err != nil {
		return fmt.Errorf("create agent indexes: %w", err)
	}

	emailIndexes := []mongo.IndexModel{
		{Keys: bson.D{
			{Key: "agent_id", Value: 1},
			{Key: "folder", Value: 1},
			{Key: "received_at", Value: -1},
		}},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	}
	if _, err := s.emails.Indexes().CreateMany(ctx, emailIndexes); err != nil {
		return fmt.Errorf("create email indexes: %w", err)
	}

	return nil
}

func (s *Store) CreateAgent(ctx context.Context, agent *Agent) error {
	_, err := s.agents.InsertOne(ctx, agent)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

func (s *Store) GetAgentByID(ctx context.Context, id string) (*Agent, error) {
	var agent Agent
	err := s.agents.FindOne(ctx, bson.M{"_id": id}).Decode(&agent)
	if err != nil {
		return nil, fmt.Errorf("find agent: %w", err)
	}
	return &agent, nil
}

func (s *Store) GetAgentByPrefix(ctx context.Context, prefix string) (*Agent, error) {
	var agent Agent
	err := s.agents.FindOne(ctx, bson.M{"api_key_prefix": prefix}).Decode(&agent)
	if err != nil {
		return nil, fmt.Errorf("find agent by prefix: %w", err)
	}
	return &agent, nil
}

func (s *Store) GetAgentByEmail(ctx context.Context, email string) (*Agent, error) {
	var agent Agent
	err := s.agents.FindOne(ctx, bson.M{"email": email}).Decode(&agent)
	if err != nil {
		return nil, fmt.Errorf("find agent by email: %w", err)
	}
	return &agent, nil
}

func (s *Store) UpdateAgent(ctx context.Context, id string, update bson.M) error {
	update["updated_at"] = time.Now().UTC()
	_, err := s.agents.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

func (s *Store) InsertEmail(ctx context.Context, email *Email) error {
	_, err := s.emails.InsertOne(ctx, email)
	if err != nil {
		return fmt.Errorf("insert email: %w", err)
	}
	return nil
}

type InboxQuery struct {
	AgentID string
	Folder  string
	Since   *time.Time
	Cursor  string
	Limit   int64
}

func (s *Store) ListEmails(ctx context.Context, q InboxQuery) ([]Email, error) {
	filter := bson.M{
		"agent_id": q.AgentID,
		"folder":   q.Folder,
	}

	if q.Since != nil {
		filter["received_at"] = bson.M{"$gte": *q.Since}
	}
	if q.Cursor != "" {
		filter["_id"] = bson.M{"$lt": q.Cursor}
	}

	now := time.Now().UTC()
	filter["$or"] = bson.A{
		bson.M{"expires_at": nil},
		bson.M{"expires_at": bson.M{"$gt": now}},
	}

	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "received_at", Value: -1}}).
		SetLimit(limit)

	cursor, err := s.emails.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find emails: %w", err)
	}
	defer cursor.Close(ctx)

	var emails []Email
	if err := cursor.All(ctx, &emails); err != nil {
		return nil, fmt.Errorf("decode emails: %w", err)
	}
	return emails, nil
}

func (s *Store) GetEmail(ctx context.Context, id, agentID string) (*Email, error) {
	var email Email
	err := s.emails.FindOne(ctx, bson.M{"_id": id, "agent_id": agentID}).Decode(&email)
	if err != nil {
		return nil, fmt.Errorf("find email: %w", err)
	}
	return &email, nil
}

func (s *Store) DeleteEmail(ctx context.Context, id, agentID string) error {
	res, err := s.emails.DeleteOne(ctx, bson.M{"_id": id, "agent_id": agentID})
	if err != nil {
		return fmt.Errorf("delete email: %w", err)
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("email not found")
	}
	return nil
}

func (s *Store) MoveEmail(ctx context.Context, id, agentID, folder string) error {
	_, err := s.emails.UpdateOne(ctx, bson.M{"_id": id, "agent_id": agentID}, bson.M{
		"$set": bson.M{"folder": folder},
	})
	if err != nil {
		return fmt.Errorf("move email: %w", err)
	}
	return nil
}
