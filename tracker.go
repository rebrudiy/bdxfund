package main

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/rebrudiy/beldex-go/walletrpc"
)

// Wallet is the subset of beldex-wallet-rpc bdxfund needs. A view-only wallet
// is enough for all of it: the server can see donations but never spend them.
type Wallet interface {
	Refresh(ctx context.Context) error
	Incoming(ctx context.Context, indices ...uint32) ([]walletrpc.Transfer, error)
	NewSubaddress(ctx context.Context, label string) (string, uint32, error)
	PrimaryAddress(ctx context.Context) (string, error)
	ViewKey(ctx context.Context) (string, error)
}

type DonationStatus string

const (
	StatusConfirmed DonationStatus = "confirmed" // in a block
	StatusFlash     DonationStatus = "flash"     // approved by the Flash quorum, not mined yet
	StatusPending   DonationStatus = "pending"   // plain mempool tx, may still fail
)

type Donation struct {
	TxID          string         `json:"txid"`
	Amount        uint64         `json:"amount"`
	Time          time.Time      `json:"time"`
	Status        DonationStatus `json:"status"`
	Confirmations uint64         `json:"confirmations"`
}

// Progress is what a campaign page shows. Raised counts confirmed + Flash;
// Pending is shown separately and never counted toward the goal.
type Progress struct {
	Raised    uint64     `json:"raised"`
	Pending   uint64     `json:"pending"`
	Donations []Donation `json:"donations"`
}

// Tracker polls the wallet and caches per-subaddress progress.
type Tracker struct {
	wallet Wallet
	store  *Store

	mu      sync.RWMutex
	bySub   map[uint32]Progress
	updated time.Time
}

func NewTracker(w Wallet, s *Store) *Tracker {
	return &Tracker{wallet: w, store: s, bySub: map[uint32]Progress{}}
}

func (t *Tracker) Progress(sub uint32) (Progress, time.Time) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bySub[sub], t.updated
}

// Run polls until ctx is cancelled.
func (t *Tracker) Run(ctx context.Context, every time.Duration) {
	for {
		if err := t.Poll(ctx); err != nil {
			log.Printf("poll: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

func (t *Tracker) Poll(ctx context.Context) error {
	campaigns := t.store.List()
	if len(campaigns) == 0 {
		return nil
	}
	if err := t.wallet.Refresh(ctx); err != nil {
		return err
	}
	indices := make([]uint32, len(campaigns))
	for i, c := range campaigns {
		indices[i] = c.SubIndex
	}
	transfers, err := t.wallet.Incoming(ctx, indices...)
	if err != nil {
		return err
	}

	bySub := aggregate(transfers)
	t.mu.Lock()
	t.bySub, t.updated = bySub, time.Now()
	t.mu.Unlock()
	return nil
}

func aggregate(transfers []walletrpc.Transfer) map[uint32]Progress {
	type key struct {
		txid string
		sub  uint32
	}
	seen := map[key]bool{} // a tx can show up in both "in" and "pool" during a race
	bySub := map[uint32]Progress{}
	for _, tr := range transfers {
		k := key{tr.TxID, tr.SubaddrIndex.Minor}
		if tr.DoubleSpend || seen[k] {
			continue
		}
		seen[k] = true

		d := Donation{TxID: tr.TxID, Amount: tr.Amount, Time: time.Unix(int64(tr.Timestamp), 0), Confirmations: tr.Confirmations}
		switch {
		case tr.Type == "in":
			d.Status = StatusConfirmed
		case tr.FlashMempool:
			d.Status = StatusFlash
		default:
			d.Status = StatusPending
		}

		p := bySub[tr.SubaddrIndex.Minor]
		if d.Status == StatusPending {
			p.Pending += d.Amount
		} else {
			p.Raised += d.Amount
		}
		p.Donations = append(p.Donations, d)
		bySub[tr.SubaddrIndex.Minor] = p
	}
	for sub, p := range bySub {
		sort.Slice(p.Donations, func(i, j int) bool { return p.Donations[i].Time.After(p.Donations[j].Time) })
		bySub[sub] = p
	}
	return bySub
}
