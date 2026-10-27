// bdxfund is a non-custodial, privacy-preserving crowdfunding server for Beldex.
//
//	bdxfund new   -id rent -title "Pay the rent" -goal 500 -desc "..."
//	bdxfund serve -addr :8080
//
// It talks to a beldex-wallet-rpc that should hold a view-only wallet.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/rebrudiy/beldex-go/walletrpc"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bdxfund new|serve [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}

type common struct {
	wallet *string
	data   *string
}

func commonFlags(fs *flag.FlagSet) common {
	return common{
		wallet: fs.String("wallet", "http://127.0.0.1:29092", "beldex-wallet-rpc URL"),
		data:   fs.String("data", "data/campaigns.json", "campaign store"),
	}
}

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	c := commonFlags(fs)
	id := fs.String("id", "", "URL slug, e.g. rent-2026")
	title := fs.String("title", "", "campaign title")
	desc := fs.String("desc", "", "description")
	goal := fs.String("goal", "", "goal in BDX, e.g. 500")
	fs.Parse(args)
	if *id == "" || *title == "" || *goal == "" {
		return fmt.Errorf("-id, -title and -goal are required")
	}
	goalAtomic, err := walletrpc.ParseBDX(*goal)
	if err != nil {
		return err
	}
	store, err := OpenStore(*c.data)
	if err != nil {
		return err
	}
	if _, dup := store.Get(*id); dup {
		return fmt.Errorf("campaign %q already exists", *id)
	}

	addr, idx, err := walletrpc.New(*c.wallet).NewSubaddress(context.Background(), "bdxfund:"+*id)
	if err != nil {
		return err
	}
	camp := Campaign{ID: *id, Title: *title, Description: *desc, Goal: goalAtomic, Address: addr, SubIndex: idx, CreatedAt: time.Now()}
	if err := store.Add(camp); err != nil {
		return err
	}
	fmt.Printf("created %s\naddress (subaddress #%d): %s\n", camp.ID, idx, addr)
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	c := commonFlags(fs)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	network := fs.String("network", "testnet", "mainnet|testnet, shown in audit instructions")
	every := fs.Duration("poll", 10*time.Second, "wallet poll interval")
	fs.Parse(args)

	store, err := OpenStore(*c.data)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	w := walletrpc.New(*c.wallet)
	primary, err := w.PrimaryAddress(ctx)
	if err != nil {
		return fmt.Errorf("wallet not reachable: %w", err)
	}
	viewKey, err := w.ViewKey(ctx)
	if err != nil {
		return err
	}

	tracker := NewTracker(w, store)
	go tracker.Run(ctx, *every)

	srv := &Server{store: store, tracker: tracker, audit: Audit{PrimaryAddress: primary, ViewKey: viewKey, Network: *network}}
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		httpSrv.Shutdown(context.Background())
	}()
	log.Printf("bdxfund on http://%s (%d campaigns)", *addr, len(store.List()))
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
