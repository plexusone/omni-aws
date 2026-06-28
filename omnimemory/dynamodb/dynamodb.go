// Package dynamodb provides a DynamoDB-backed Provider implementation for omnimemory.
//
// This provider stores memories in a DynamoDB table with automatic TTL support
// and in-memory vector search for semantic operations.
//
// # Table Schema
//
// The provider expects a DynamoDB table with the following schema:
//
//   - Partition Key (pk): String - tenant_id
//   - Sort Key (sk): String - subject_id#memory_id
//   - TTL Attribute: expires_at (Unix timestamp)
//
// # Configuration Options
//
//   - table_name: DynamoDB table name (required)
//   - region: AWS region (optional, uses default config if not set)
//   - endpoint: Custom endpoint URL for local development (optional)
//   - create_table: Auto-create table if not exists (default: false)
//
// # Usage
//
//	import (
//	    "github.com/plexusone/omnimemory"
//	    "github.com/plexusone/omnimemory/core"
//	    _ "github.com/plexusone/omni-aws/omnimemory/dynamodb"
//	)
//
//	client, err := omnimemory.NewClient(core.ClientConfig{
//	    Providers: []core.ProviderConfig{
//	        {
//	            Name: core.ProviderNameAWSDynamoDB,
//	            Options: map[string]any{
//	                "table_name": "omnimemory",
//	            },
//	        },
//	    },
//	})
package dynamodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/plexusone/omnimemory/core"
)

func init() {
	core.RegisterProvider(core.ProviderNameAWSDynamoDB, NewProvider, core.PriorityThick)
}

// memoryItem represents a memory stored in DynamoDB.
type memoryItem struct {
	PK        string  `dynamodbav:"pk"`         // tenant_id
	SK        string  `dynamodbav:"sk"`         // subject_id#memory_id
	ID        string  `dynamodbav:"id"`         // memory_id
	TenantID  string  `dynamodbav:"tenant_id"`  // For GSI queries
	SubjectID string  `dynamodbav:"subject_id"` // For filtering
	AgentID   string  `dynamodbav:"agent_id,omitempty"`
	SessionID string  `dynamodbav:"session_id,omitempty"`
	Scope     string  `dynamodbav:"scope"`
	Type      string  `dynamodbav:"type"`
	Content   string  `dynamodbav:"content"`
	Embedding string  `dynamodbav:"embedding,omitempty"` // JSON-encoded []float64
	Metadata  string  `dynamodbav:"metadata,omitempty"`  // JSON-encoded map
	CreatedAt int64   `dynamodbav:"created_at"`          // Unix timestamp
	UpdatedAt int64   `dynamodbav:"updated_at"`          // Unix timestamp
	ExpiresAt *int64  `dynamodbav:"expires_at,omitempty"` // TTL attribute
	TypeSort  string  `dynamodbav:"type_sort,omitempty"`  // type#created_at for GSI
	ScopeSort string  `dynamodbav:"scope_sort,omitempty"` // scope#created_at for GSI
}

// Provider implements core.Provider using DynamoDB.
type Provider struct {
	client    *dynamodb.Client
	tableName string
	embedder  core.Embedder
}

// NewProvider creates a new DynamoDB Provider.
func NewProvider(cfg core.ProviderConfig, embedder core.Embedder) (core.Provider, error) {
	tableName, ok := cfg.Options["table_name"].(string)
	if !ok || tableName == "" {
		return nil, core.NewValidationError("table_name", "table_name is required")
	}

	// Build AWS config
	var optFns []func(*config.LoadOptions) error

	if region, ok := cfg.Options["region"].(string); ok && region != "" {
		optFns = append(optFns, config.WithRegion(region))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		return nil, fmt.Errorf("dynamodb: loading AWS config: %w", err)
	}

	// Build DynamoDB client options
	var clientOpts []func(*dynamodb.Options)

	if endpoint, ok := cfg.Options["endpoint"].(string); ok && endpoint != "" {
		clientOpts = append(clientOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	client := dynamodb.NewFromConfig(awsCfg, clientOpts...)

	p := &Provider{
		client:    client,
		tableName: tableName,
		embedder:  embedder,
	}

	// Optionally create table
	if createTable, ok := cfg.Options["create_table"].(bool); ok && createTable {
		if err := p.createTableIfNotExists(context.Background()); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return core.ProviderNameAWSDynamoDB.String()
}

// Close closes the provider.
func (p *Provider) Close() error {
	return nil
}

// Add adds a new memory.
func (p *Provider) Add(ctx context.Context, req *core.AddRequest) (*core.Memory, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	now := time.Now()
	id := uuid.New().String()

	memory := &core.Memory{
		ID:        id,
		TenantID:  req.TenantID,
		SubjectID: req.SubjectID,
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		Scope:     req.Scope,
		Type:      req.Type,
		Content:   req.Content,
		Metadata:  req.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Set expiration if TTL is provided
	if req.TTL > 0 {
		expiresAt := now.Add(req.TTL)
		memory.ExpiresAt = &expiresAt
	}

	// Generate embedding if embedder is available
	if p.embedder != nil {
		embedding, err := p.embedder.Embed(ctx, req.Content)
		if err != nil {
			return nil, core.NewProviderError(p.Name(), "Add", err)
		}
		memory.Embedding = embedding
	}

	// Convert to DynamoDB item
	item, err := p.memoryToItem(memory)
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Add", err)
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Add", err)
	}

	_, err = p.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(p.tableName),
		Item:      av,
	})
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Add", err)
	}

	return memory, nil
}

// Get retrieves a memory by ID.
func (p *Provider) Get(ctx context.Context, req *core.GetRequest) (*core.Memory, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	pk := req.TenantID
	sk := req.SubjectID + "#" + req.ID

	result, err := p.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(p.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Get", err)
	}

	if result.Item == nil {
		return nil, core.ErrNotFound
	}

	var item memoryItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, core.NewProviderError(p.Name(), "Get", err)
	}

	memory, err := p.itemToMemory(&item)
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Get", err)
	}

	// Check expiration
	if memory.ExpiresAt != nil && time.Now().After(*memory.ExpiresAt) {
		return nil, core.ErrNotFound
	}

	return memory, nil
}

// Update updates an existing memory.
func (p *Provider) Update(ctx context.Context, req *core.UpdateRequest) (*core.Memory, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get existing memory
	existing, err := p.Get(ctx, &core.GetRequest{
		Context: req.Context,
		ID:      req.ID,
	})
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Content != "" {
		existing.Content = req.Content

		// Regenerate embedding if content changed
		if p.embedder != nil {
			embedding, err := p.embedder.Embed(ctx, req.Content)
			if err != nil {
				return nil, core.NewProviderError(p.Name(), "Update", err)
			}
			existing.Embedding = embedding
		}
	}

	if req.Metadata != nil {
		if existing.Metadata == nil {
			existing.Metadata = make(map[string]any)
		}
		for k, v := range req.Metadata {
			existing.Metadata[k] = v
		}
	}

	existing.UpdatedAt = time.Now()

	// Convert to DynamoDB item and save
	item, err := p.memoryToItem(existing)
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Update", err)
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Update", err)
	}

	_, err = p.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(p.tableName),
		Item:      av,
	})
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "Update", err)
	}

	return existing, nil
}

// Delete deletes a memory by ID.
func (p *Provider) Delete(ctx context.Context, req *core.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	pk := req.TenantID
	sk := req.SubjectID + "#" + req.ID

	_, err := p.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(p.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return core.NewProviderError(p.Name(), "Delete", err)
	}

	return nil
}

// List lists memories with optional filters.
func (p *Provider) List(ctx context.Context, req *core.ListRequest) (*core.ListResponse, error) {
	if req.TenantID == "" {
		return nil, core.ErrTenantRequired
	}

	// Build query
	keyCondition := "pk = :pk"
	exprAttrValues := map[string]types.AttributeValue{
		":pk": &types.AttributeValueMemberS{Value: req.TenantID},
	}

	// Add subject filter if provided
	if req.SubjectID != "" {
		keyCondition += " AND begins_with(sk, :sk_prefix)"
		exprAttrValues[":sk_prefix"] = &types.AttributeValueMemberS{Value: req.SubjectID + "#"}
	}

	limit := int32(req.Limit)
	if limit <= 0 {
		limit = 100
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(p.tableName),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: exprAttrValues,
		Limit:                     aws.Int32(limit + 1), // +1 to check HasMore
		ScanIndexForward:          aws.Bool(false),      // Most recent first
	}

	result, err := p.client.Query(ctx, queryInput)
	if err != nil {
		return nil, core.NewProviderError(p.Name(), "List", err)
	}

	var memories []*core.Memory
	now := time.Now()

	for _, itemAV := range result.Items {
		var item memoryItem
		if err := attributevalue.UnmarshalMap(itemAV, &item); err != nil {
			continue
		}

		memory, err := p.itemToMemory(&item)
		if err != nil {
			continue
		}

		// Check expiration
		if memory.ExpiresAt != nil && now.After(*memory.ExpiresAt) {
			continue
		}

		// Filter by types
		if len(req.Types) > 0 && !containsType(req.Types, memory.Type) {
			continue
		}

		// Filter by scopes
		if len(req.Scopes) > 0 && !containsScope(req.Scopes, memory.Scope) {
			continue
		}

		memories = append(memories, memory)
	}

	hasMore := len(memories) > int(limit)
	if hasMore {
		memories = memories[:limit]
	}

	return &core.ListResponse{
		Memories:   memories,
		TotalCount: len(memories),
		HasMore:    hasMore,
	}, nil
}

// Search performs semantic search on memories.
func (p *Provider) Search(ctx context.Context, req *core.SearchRequest) (*core.SearchResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Generate embedding for query
	var queryEmbedding []float64
	if p.embedder != nil {
		embedding, err := p.embedder.Embed(ctx, req.Query)
		if err != nil {
			return nil, core.NewProviderError(p.Name(), "Search", err)
		}
		queryEmbedding = embedding
	}

	// List all memories for this tenant/subject
	listResp, err := p.List(ctx, &core.ListRequest{
		Context: req.Context,
		Types:   req.Types,
		Scopes:  req.Scopes,
		Limit:   0, // Get all for scoring
	})
	if err != nil {
		return nil, err
	}

	type scoredMemory struct {
		memory *core.Memory
		score  float64
	}

	var scored []scoredMemory

	for _, memory := range listResp.Memories {
		// Calculate similarity score
		var score float64
		if len(queryEmbedding) > 0 && len(memory.Embedding) > 0 {
			score = core.CosineSimilarity(queryEmbedding, memory.Embedding)
		}

		// Apply threshold
		if req.Threshold > 0 && score < req.Threshold {
			continue
		}

		scored = append(scored, scoredMemory{memory: memory, score: score})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Apply limit
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]*core.SearchResult, len(scored))
	for i, sm := range scored {
		results[i] = &core.SearchResult{
			Memory: sm.memory,
			Score:  sm.score,
		}
	}

	return &core.SearchResponse{
		Results: results,
	}, nil
}

// Recall retrieves relevant memories for a given query.
func (p *Provider) Recall(ctx context.Context, req *core.RecallRequest) (*core.RecallResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Use Search under the hood
	searchReq := &core.SearchRequest{
		Context: req.Context,
		Query:   req.Query,
		Types:   req.IncludeTypes,
		Limit:   req.MaxResults,
	}

	searchResp, err := p.Search(ctx, searchReq)
	if err != nil {
		return nil, err
	}

	memories := make([]*core.Memory, len(searchResp.Results))
	for i, r := range searchResp.Results {
		memories[i] = r.Memory
	}

	return &core.RecallResponse{
		Memories: memories,
	}, nil
}

// memoryToItem converts a core.Memory to a DynamoDB item.
func (p *Provider) memoryToItem(m *core.Memory) (*memoryItem, error) {
	item := &memoryItem{
		PK:        m.TenantID,
		SK:        m.SubjectID + "#" + m.ID,
		ID:        m.ID,
		TenantID:  m.TenantID,
		SubjectID: m.SubjectID,
		AgentID:   m.AgentID,
		SessionID: m.SessionID,
		Scope:     string(m.Scope),
		Type:      string(m.Type),
		Content:   m.Content,
		CreatedAt: m.CreatedAt.Unix(),
		UpdatedAt: m.UpdatedAt.Unix(),
		TypeSort:  string(m.Type) + "#" + strconv.FormatInt(m.CreatedAt.Unix(), 10),
		ScopeSort: string(m.Scope) + "#" + strconv.FormatInt(m.CreatedAt.Unix(), 10),
	}

	// Encode embedding as JSON
	if len(m.Embedding) > 0 {
		embJSON, err := json.Marshal(m.Embedding)
		if err != nil {
			return nil, err
		}
		item.Embedding = string(embJSON)
	}

	// Encode metadata as JSON
	if len(m.Metadata) > 0 {
		metaJSON, err := json.Marshal(m.Metadata)
		if err != nil {
			return nil, err
		}
		item.Metadata = string(metaJSON)
	}

	// Set TTL
	if m.ExpiresAt != nil {
		ts := m.ExpiresAt.Unix()
		item.ExpiresAt = &ts
	}

	return item, nil
}

// itemToMemory converts a DynamoDB item to a core.Memory.
func (p *Provider) itemToMemory(item *memoryItem) (*core.Memory, error) {
	memory := &core.Memory{
		ID:        item.ID,
		TenantID:  item.TenantID,
		SubjectID: item.SubjectID,
		AgentID:   item.AgentID,
		SessionID: item.SessionID,
		Scope:     core.Scope(item.Scope),
		Type:      core.MemoryType(item.Type),
		Content:   item.Content,
		CreatedAt: time.Unix(item.CreatedAt, 0),
		UpdatedAt: time.Unix(item.UpdatedAt, 0),
	}

	// Decode embedding from JSON
	if item.Embedding != "" {
		var embedding []float64
		if err := json.Unmarshal([]byte(item.Embedding), &embedding); err != nil {
			return nil, err
		}
		memory.Embedding = embedding
	}

	// Decode metadata from JSON
	if item.Metadata != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(item.Metadata), &metadata); err != nil {
			return nil, err
		}
		memory.Metadata = metadata
	}

	// Set expiration
	if item.ExpiresAt != nil {
		t := time.Unix(*item.ExpiresAt, 0)
		memory.ExpiresAt = &t
	}

	return memory, nil
}

// createTableIfNotExists creates the DynamoDB table if it doesn't exist.
func (p *Provider) createTableIfNotExists(ctx context.Context) error {
	// Check if table exists
	_, err := p.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(p.tableName),
	})
	if err == nil {
		return nil // Table exists
	}

	// Check if error is ResourceNotFoundException
	var rnf *types.ResourceNotFoundException
	if !errors.As(err, &rnf) {
		return fmt.Errorf("dynamodb: checking table: %w", err)
	}

	// Create table
	_, err = p.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(p.tableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return fmt.Errorf("dynamodb: creating table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(p.client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(p.tableName),
	}, 2*time.Minute)
	if err != nil {
		return fmt.Errorf("dynamodb: waiting for table: %w", err)
	}

	// Enable TTL
	_, err = p.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(p.tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("expires_at"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		// TTL update may fail if already enabled, ignore that case
		if !strings.Contains(err.Error(), "already enabled") {
			return fmt.Errorf("dynamodb: enabling TTL: %w", err)
		}
	}

	return nil
}

// containsType checks if a slice contains a memory type.
func containsType(types []core.MemoryType, t core.MemoryType) bool {
	for _, mt := range types {
		if mt == t {
			return true
		}
	}
	return false
}

// containsScope checks if a slice contains a scope.
func containsScope(scopes []core.Scope, s core.Scope) bool {
	for _, sc := range scopes {
		if sc == s {
			return true
		}
	}
	return false
}
