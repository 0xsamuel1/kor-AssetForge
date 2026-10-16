package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEventIndexer_SyncAndProcess(t *testing.T) {
	// Set up database
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.IndexedEvent{}, &models.EventCheckpoint{})
	require.NoError(t, err)

	// Mock RPC response
	mockEvents := []SorobanEvent{
		{
			Type:           "contract",
			Ledger:         100,
			LedgerClosedAt: time.Now().Format(time.RFC3339),
			ID:             "0000000000000000100-0000000001",
			ContractID:     "C1111111111111111111111111111111111111111111111111111111",
			Topic:          []string{"transfer", "alice", "bob"},
			TxHash:         "txhash123",
		},
	}
	mockEvents[0].Value.XDR = "AAAAAAA="

	var handlerCalled bool
	var handlerMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	// Start mock RPC server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req GetEventsRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		resp := GetEventsResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: &GetEventsResult{
				LatestLedger: 105,
				Events:       mockEvents,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	// Initialize EventIndexer
	indexer := NewEventIndexer(db)
	indexer.rpcURL = server.URL
	indexer.pollInterval = 100 * time.Millisecond

	// Register event handler
	indexer.RegisterHandler("C1111111111111111111111111111111111111111111111111111111", "transfer", func(ctx context.Context, ev *models.IndexedEvent) error {
		handlerMu.Lock()
		defer handlerMu.Unlock()
		handlerCalled = true
		wg.Done()
		return nil
	})

	// Start indexing
	indexer.Start()
	defer indexer.Stop()

	// Wait for handler to be invoked
	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for handler to be called")
	}

	handlerMu.Lock()
	assert.True(t, handlerCalled)
	handlerMu.Unlock()

	// Check database records
	var ev models.IndexedEvent
	err = db.Where("event_id = ?", mockEvents[0].ID).First(&ev).Error
	require.NoError(t, err)
	assert.Equal(t, models.EventStatusProcessed, ev.Status)
	assert.Equal(t, uint32(100), ev.Ledger)
	assert.Equal(t, "txhash123", ev.TxHash)

	// Check checkpoint
	var cp models.EventCheckpoint
	err = db.Where("checkpoint = ?", "global_stellar_indexer").First(&cp).Error
	require.NoError(t, err)
	assert.Equal(t, uint32(105), cp.LastLedger)
}
