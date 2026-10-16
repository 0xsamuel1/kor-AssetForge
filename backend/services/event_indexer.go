package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type EventHandler func(ctx context.Context, event *models.IndexedEvent) error

type EventIndexer struct {
	db           *gorm.DB
	rpcURL       string
	pollInterval time.Duration
	maxRetries   int
	handlers     map[string]map[string]EventHandler // contractID -> topic -> handler
	handlersMu   sync.RWMutex
	eventQueue   chan *models.IndexedEvent
	workersCount int
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
	runningMu    sync.Mutex
}

type GetEventsRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  GetEventsParams `json:"params"`
}

type GetEventsParams struct {
	StartLedger uint32             `json:"startLedger"`
	Filters     []GetEventsFilter  `json:"filters,omitempty"`
	Limit       int                `json:"limit,omitempty"`
}

type GetEventsFilter struct {
	Type        string   `json:"type"`
	ContractIds []string `json:"contractIds,omitempty"`
	Topics      []string `json:"topics,omitempty"`
}

type GetEventsResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int              `json:"id"`
	Result  *GetEventsResult `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type GetEventsResult struct {
	LatestLedger uint32        `json:"latestLedger"`
	Events       []SorobanEvent `json:"events"`
}

type SorobanEvent struct {
	Type           string   `json:"type"`
	Ledger         uint32   `json:"ledger"`
	LedgerClosedAt string   `json:"ledgerClosedAt"`
	ID             string   `json:"id"`
	ContractID     string   `json:"contractId"`
	Topic          []string `json:"topic"`
	Value          struct {
		XDR string `json:"xdr"`
	} `json:"value"`
	TxHash string `json:"txHash"`
}

// NewEventIndexer creates a new EventIndexer instance
func NewEventIndexer(db *gorm.DB) *EventIndexer {
	rpcURL := os.Getenv("STELLAR_RPC_URL")
	if rpcURL == "" {
		rpcURL = "https://soroban-testnet.stellar.org" // fallback
	}

	pollStr := os.Getenv("EVENT_INDEXER_POLL_INTERVAL")
	pollInterval := 5 * time.Second
	if pollStr != "" {
		if d, err := time.ParseDuration(pollStr); err == nil {
			pollInterval = d
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &EventIndexer{
		db:           db,
		rpcURL:       rpcURL,
		pollInterval: pollInterval,
		maxRetries:   5,
		handlers:     make(map[string]map[string]EventHandler),
		eventQueue:   make(chan *models.IndexedEvent, 1000),
		workersCount: 3,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// RegisterHandler registers a handler for specific contract events
func (ei *EventIndexer) RegisterHandler(contractID string, topic string, handler EventHandler) {
	ei.handlersMu.Lock()
	defer ei.handlersMu.Unlock()

	if _, ok := ei.handlers[contractID]; !ok {
		ei.handlers[contractID] = make(map[string]EventHandler)
	}
	ei.handlers[contractID][topic] = handler
}

// Start runs the indexer service in the background
func (ei *EventIndexer) Start() {
	ei.runningMu.Lock()
	if ei.running {
		ei.runningMu.Unlock()
		return
	}
	ei.running = true
	ei.runningMu.Unlock()

	log.Printf("EventIndexer: Starting indexer with RPC %s", ei.rpcURL)

	// Start workers
	for i := 0; i < ei.workersCount; i++ {
		ei.wg.Add(1)
		go ei.worker(i)
	}

	// Start poller
	ei.wg.Add(1)
	go ei.poller()
}

// Stop stops the indexer service
func (ei *EventIndexer) Stop() {
	ei.runningMu.Lock()
	if !ei.running {
		ei.runningMu.Unlock()
		return
	}
	ei.running = false
	ei.runningMu.Unlock()

	log.Println("EventIndexer: Stopping indexer...")
	ei.cancel()
	close(ei.eventQueue)
	ei.wg.Wait()
	log.Println("EventIndexer: Indexer stopped.")
}

// getCheckpoint loads or creates the checkpoint record
func (ei *EventIndexer) getCheckpoint() (*models.EventCheckpoint, error) {
	var cp models.EventCheckpoint
	err := ei.db.Where("checkpoint = ?", "global_stellar_indexer").First(&cp).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Get current network ledger or start from 1
			startLedger := uint32(1)
			cp = models.EventCheckpoint{
				Checkpoint: "global_stellar_indexer",
				LastLedger: startLedger,
				Cursor:     "",
			}
			if createErr := ei.db.Create(&cp).Error; createErr != nil {
				return nil, createErr
			}
			return &cp, nil
		}
		return nil, err
	}
	return &cp, nil
}

// updateCheckpoint saves the checkpoint record
func (ei *EventIndexer) updateCheckpoint(ledger uint32, cursor string) error {
	return ei.db.Model(&models.EventCheckpoint{}).
		Where("checkpoint = ?", "global_stellar_indexer").
		Updates(map[string]interface{}{
			"last_ledger": ledger,
			"cursor":      cursor,
			"updated_at":  time.Now(),
		}).Error
}

func (ei *EventIndexer) poller() {
	defer ei.wg.Done()

	ticker := time.NewTicker(ei.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ei.ctx.Done():
			return
		case <-ticker.C:
			checkpoint, err := ei.getCheckpoint()
			if err != nil {
				log.Printf("EventIndexer Error: Failed to fetch checkpoint: %v", err)
				continue
			}

			events, latestRPCledger, err := ei.fetchEvents(checkpoint.LastLedger)
			if err != nil {
				log.Printf("EventIndexer Warning: Failed to fetch events from RPC: %v", err)
				continue
			}

			if len(events) > 0 {
				log.Printf("EventIndexer: Fetched %d new blockchain events", len(events))
			}

			lastProcessedLedger := checkpoint.LastLedger
			for _, sev := range events {
				// Convert to GORM model
				topicJSON, _ := json.Marshal(sev.Topic)
				closedAt, parseErr := time.Parse(time.RFC3339, sev.LedgerClosedAt)
				if parseErr != nil {
					closedAt = time.Now()
				}

				dbEvent := &models.IndexedEvent{
					ContractID:     sev.ContractID,
					Ledger:         sev.Ledger,
					LedgerClosedAt: closedAt,
					TxHash:         sev.TxHash,
					EventID:        sev.ID,
					Topic:          string(topicJSON),
					Value:          sev.Value.XDR,
					Status:         models.EventStatusPending,
				}

				// Check for duplicates
				var existing models.IndexedEvent
				if err := ei.db.Where("event_id = ?", dbEvent.EventID).First(&existing).Error; err == nil {
					// Duplicate event, skip
					continue
				}

				// Save to database
				if err := ei.db.Create(dbEvent).Error; err != nil {
					log.Printf("EventIndexer Error: Failed to save event to DB: %v", err)
					continue
				}

				// Enqueue for processing
				select {
				case ei.eventQueue <- dbEvent:
				case <-ei.ctx.Done():
					return
				}

				if sev.Ledger > lastProcessedLedger {
					lastProcessedLedger = sev.Ledger
				}
			}

			// Update checkpoint to either the last processed ledger or the latest ledger returned by RPC
			targetLedger := lastProcessedLedger
			if latestRPCledger > targetLedger {
				targetLedger = latestRPCledger
			}
			if targetLedger > checkpoint.LastLedger {
				if err := ei.updateCheckpoint(targetLedger, ""); err != nil {
					log.Printf("EventIndexer Error: Failed to update checkpoint: %v", err)
				}
			}
		}
	}
}

func (ei *EventIndexer) fetchEvents(startLedger uint32) ([]SorobanEvent, uint32, error) {
	// Construct the JSON-RPC request payload
	reqBody := GetEventsRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "getEvents",
		Params: GetEventsParams{
			StartLedger: startLedger,
			Limit:       100,
		},
	}

	// Fetch contract IDs to filter, if any are registered
	ei.handlersMu.RLock()
	var contractIDs []string
	for cid := range ei.handlers {
		contractIDs = append(contractIDs, cid)
	}
	ei.handlersMu.RUnlock()

	if len(contractIDs) > 0 {
		reqBody.Params.Filters = []GetEventsFilter{
			{
				Type:        "contract",
				ContractIds: contractIDs,
			},
		}
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ei.ctx, "POST", ei.rpcURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var rpcResp GetEventsResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, 0, err
	}

	if rpcResp.Error != nil {
		return nil, 0, fmt.Errorf("RPC error (%d): %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if rpcResp.Result == nil {
		return nil, 0, nil
	}

	return rpcResp.Result.Events, rpcResp.Result.LatestLedger, nil
}

func (ei *EventIndexer) worker(id int) {
	defer ei.wg.Done()

	for ev := range ei.eventQueue {
		ei.processEvent(ev)
	}
}

func (ei *EventIndexer) processEvent(ev *models.IndexedEvent) {
	// Parse topics to identify correct handler
	var topics []string
	if err := json.Unmarshal([]byte(ev.Topic), &topics); err != nil {
		ei.markFailed(ev, fmt.Sprintf("failed to parse topic JSON: %v", err))
		return
	}

	if len(topics) == 0 {
		ei.markProcessed(ev)
		return
	}

	primaryTopic := topics[0]

	ei.handlersMu.RLock()
	var handler EventHandler
	if contractHandlers, ok := ei.handlers[ev.ContractID]; ok {
		handler = contractHandlers[primaryTopic]
	}
	ei.handlersMu.RUnlock()

	if handler == nil {
		// No handler registered for this event topic, mark processed anyway
		ei.markProcessed(ev)
		return
	}

	// Process event with retry logic
	var processErr error
	backoff := 500 * time.Millisecond

	for i := 0; i <= ei.maxRetries; i++ {
		if i > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		ctx, cancel := context.WithTimeout(ei.ctx, 10*time.Second)
		processErr = handler(ctx, ev)
		cancel()

		if processErr == nil {
			break
		}

		log.Printf("EventIndexer: Worker failed to process event %s (attempt %d/%d): %v", ev.EventID, i+1, ei.maxRetries+1, processErr)
	}

	if processErr != nil {
		ei.markFailed(ev, processErr.Error())
	} else {
		ei.markProcessed(ev)
	}
}

func (ei *EventIndexer) markProcessed(ev *models.IndexedEvent) {
	err := ei.db.Model(ev).Updates(map[string]interface{}{
		"status":     models.EventStatusProcessed,
		"updated_at": time.Now(),
	}).Error
	if err != nil {
		log.Printf("EventIndexer Error: Failed to update event status: %v", err)
	}
}

func (ei *EventIndexer) markFailed(ev *models.IndexedEvent, errMsg string) {
	err := ei.db.Model(ev).Updates(map[string]interface{}{
		"status":        models.EventStatusFailed,
		"retry_count":   ev.RetryCount + 1,
		"error_details": errMsg,
		"updated_at":    time.Now(),
	}).Error
	if err != nil {
		log.Printf("EventIndexer Error: Failed to update failed event status: %v", err)
	}
}
