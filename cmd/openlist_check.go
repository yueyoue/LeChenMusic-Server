package cmd

import (
	"context"
	"sort"
	"time"

	"github.com/navidrome/navidrome/adapters/openlist"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

// OpenList gateway connectivity self-test (评审 §4.6 P2-5).
//
// Every declared gateway is checked once at startup with a single login + root listing,
// so a broken/misconfigured gateway is visible in the logs immediately — instead of
// surfacing as "the cloud library scanned 0 books" much later. It never blocks or fails
// startup: a gateway being down is an operational problem to report, not a reason to
// refuse to serve local libraries.

// startOpenListCheck runs the self-test against every configured gateway (sorted by name
// for deterministic logs).
func startOpenListCheck(ctx context.Context) func() error {
	return func() error {
		gateways := conf.Server.OpenList
		if len(gateways) == 0 {
			return nil
		}
		names := make([]string, 0, len(gateways))
		for name := range gateways {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			opts := gateways[name]
			if err := checkOpenListGateway(ctx, name, opts); err != nil {
				log.Error(ctx, "OpenList gateway self-test FAILED — cloud libraries on this gateway will not work", "gateway", name, "url", opts.URL, err)
			}
		}
		return nil
	}
}

// checkOpenListGateway performs one login + root listing against a gateway and logs the
// outcome. Timeouts are bounded so a hung gateway cannot stall startup.
func checkOpenListGateway(ctx context.Context, name string, opts conf.OpenListOptions) error {
	client := openlist.NewClient(openlist.Config{
		BaseURL:  opts.URL,
		Username: opts.Username,
		Password: opts.Password,
		Token:    opts.Token,
		// A self-test is one probe, not a scan: keep retries minimal so a broken
		// gateway is reported quickly and without extra traffic (风控).
		MaxRetries:       1,
		FailureThreshold: 1,
	})

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	entries, err := client.List(cctx, "/", 1)
	if err != nil {
		return err
	}
	log.Info(ctx, "OpenList gateway self-test OK", "gateway", name, "url", opts.URL, "rootEntries", len(entries))
	return nil
}
