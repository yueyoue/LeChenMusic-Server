package cmd

import (
	"context"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/cloudsource"
	"github.com/navidrome/navidrome/log"
)

// OpenList gateway connectivity self-test (评审 §4.6 P2-5).
//
// Every declared gateway is checked once at startup with a single login + root listing,
// so a broken/misconfigured gateway is visible in the logs immediately — instead of
// surfacing as "the cloud library scanned 0 books" much later. It never blocks or fails
// startup: a gateway being down is an operational problem to report, not a reason to
// refuse to serve local libraries.
//
// The result is also recorded (via cloudsource.ProbeGateway) so the 云源管理面板
// (评审 P2-4) can show the startup check outcome without re-probing every page view.

// startOpenListCheck runs the self-test against every configured gateway (sorted by name
// for deterministic logs).
func startOpenListCheck(ctx context.Context) func() error {
	return func() error {
		for _, name := range cloudsource.SortedGatewayNames() {
			result, err := cloudsource.ProbeGateway(ctx, name, "startup")
			if err != nil {
				log.Error(ctx, "OpenList gateway self-test FAILED — cloud libraries on this gateway will not work",
					"gateway", name, "url", conf.Server.OpenList[name].URL, err)
				continue
			}
			log.Info(ctx, "OpenList gateway self-test OK",
				"gateway", name, "url", conf.Server.OpenList[name].URL, "rootEntries", result.RootEntries)
		}
		return nil
	}
}
