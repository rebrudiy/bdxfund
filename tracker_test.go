package main

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebrudiy/beldex-go/walletrpc"
)

type fakeWallet struct{ transfers []walletrpc.Transfer }

func (f *fakeWallet) Refresh(context.Context) error { return nil }
func (f *fakeWallet) Incoming(_ context.Context, _ ...uint32) ([]walletrpc.Transfer, error) {
	return f.transfers, nil
}
func (f *fakeWallet) NewSubaddress(context.Context, string) (string, uint32, error) {
	return "bxSub", 1, nil
}
func (f *fakeWallet) PrimaryAddress(context.Context) (string, error) { return "bxPrimary", nil }
func (f *fakeWallet) ViewKey(context.Context) (string, error)        { return "vk", nil }

func sub(minor uint32) walletrpc.SubaddressIndex { return walletrpc.SubaddressIndex{Minor: minor} }

func TestAggregate(t *testing.T) {
	bdx := uint64(walletrpc.AtomicPerBDX)
	got := aggregate([]walletrpc.Transfer{
		{TxID: "a", Amount: 10 * bdx, Type: "in", SubaddrIndex: sub(1), Timestamp: 1},
		{TxID: "b", Amount: 5 * bdx, Type: "pool", FlashMempool: true, SubaddrIndex: sub(1), Timestamp: 2},
		{TxID: "c", Amount: 3 * bdx, Type: "pool", SubaddrIndex: sub(1), Timestamp: 3},
		{TxID: "d", Amount: 100 * bdx, Type: "pool", DoubleSpend: true, SubaddrIndex: sub(1)},
		{TxID: "a", Amount: 10 * bdx, Type: "in", SubaddrIndex: sub(1)}, // duplicate
		{TxID: "e", Amount: 7 * bdx, Type: "in", SubaddrIndex: sub(2)},
	})

	p1 := got[1]
	if p1.Raised != 15*bdx {
		t.Errorf("raised = %s, want 15 (confirmed + flash)", walletrpc.FormatBDX(p1.Raised))
	}
	if p1.Pending != 3*bdx {
		t.Errorf("pending = %s, want 3", walletrpc.FormatBDX(p1.Pending))
	}
	if len(p1.Donations) != 3 || p1.Donations[0].TxID != "c" {
		t.Errorf("donations = %+v, want 3 newest-first", p1.Donations)
	}
	if got[2].Raised != 7*bdx {
		t.Errorf("campaigns leaked into each other: %+v", got)
	}
}

func TestCampaignPage(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	bdx := uint64(walletrpc.AtomicPerBDX)
	if err := store.Add(Campaign{ID: "rent", Title: "Pay the rent", Goal: 20 * bdx, Address: "bxSub", SubIndex: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	w := &fakeWallet{transfers: []walletrpc.Transfer{{TxID: "a", Amount: 5 * bdx, Type: "in", SubaddrIndex: sub(1)}}}
	tr := NewTracker(w, store)
	if err := tr.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}

	srv := &Server{store: store, tracker: tr, audit: Audit{PrimaryAddress: "bxPrimary", ViewKey: "vk", Network: "testnet"}}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/c/rent", nil))
	body := rec.Body.String()
	for _, want := range []string{"Pay the rent", "<b>5</b> of 20 BDX", "width:25%", "bxSub", "vk", "--testnet"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/c/nope", nil))
	if rec.Code != 404 {
		t.Errorf("unknown campaign: code %d, want 404", rec.Code)
	}
}

func TestStoreRejectsBadIDs(t *testing.T) {
	store, _ := OpenStore(filepath.Join(t.TempDir(), "c.json"))
	for _, id := range []string{"", "A", "../x", "has space"} {
		if err := store.Add(Campaign{ID: id}); err == nil {
			t.Errorf("Add(%q) succeeded", id)
		}
	}
}
