package scanner_test

import (
	"context"
	"path/filepath"
	"testing/fstest"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/core/artwork"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/core/storage/storagetest"
	"github.com/navidrome/navidrome/db"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/persistence"
	"github.com/navidrome/navidrome/scanner"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/tests"
	"github.com/navidrome/navidrome/utils/slice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// 评审 P2-4 (云源管理面板 启停): a gateway disabled from the admin panel must be skipped
// by scans entirely — no listing, no import, and no missing/purge processing for its
// libraries. Local libraries keep scanning normally (红线: 本地库行为零变化).
var _ = Describe("Scanner - disabled cloud gateway", Ordered, func() {
	var ctx context.Context
	var localLib, cloudLib model.Library
	var ds *tests.MockDataStore
	var s model.Scanner

	BeforeAll(func() {
		tests.SkipOnWindows("SQLite file lock blocks TempDir cleanup (#TBD-path-sep-scanner)")
		ctx = request.WithUser(GinkgoT().Context(), model.User{ID: "123", IsAdmin: true})
		tmpDir := GinkgoT().TempDir()
		conf.Server.DbPath = filepath.Join(tmpDir, "test-scanner-gateway-skip.db?_journal_mode=WAL")
		log.Warn("Using DB at " + conf.Server.DbPath)
		db.Db().SetMaxOpenConns(1)
	})

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		conf.Server.DevExternalScanner = false
		// The default library (auto-created with MusicFolder) needs a registered storage
		// scheme too — point it at an empty fake FS (same trick as scanner_multilibrary_test).
		conf.Server.MusicFolder = "defaultgw:///music"
		emptyFS := storagetest.FakeFS{}
		emptyFS.SetFiles(fstest.MapFS{})
		storagetest.Register("defaultgw", &emptyFS)
		// A gateway pointing at a closed local port: any accidental contact fails instantly
		// (connection refused), so a test run can never hang on network timeouts.
		conf.Server.OpenList = map[string]conf.OpenListOptions{
			"gw": {URL: "http://127.0.0.1:1", Token: "t"},
		}

		fs := storagetest.FakeFS{}
		fs.SetFiles(fstest.MapFS{
			"Artist/Album/01 - Track.mp3": template(_t{"albumartist": "Artist", "album": "Album"})(track(1, "Track")),
		})
		storagetest.Register("localgw", &fs)

		db.Init(ctx)
		DeferCleanup(func() {
			Expect(tests.ClearDB()).To(Succeed())
		})

		ds = &tests.MockDataStore{RealDS: persistence.New(db.Db())}
		adminUser := model.User{ID: "123", UserName: "admin", Name: "Admin User", IsAdmin: true, NewPassword: "password"}
		Expect(ds.User(ctx).Put(&adminUser)).To(Succeed())

		s = scanner.New(ctx, ds, artwork.NoopCacheWarmer(), events.NoopBroker(),
			playlists.NewPlaylists(ds, core.NewImageUploadService()), metrics.NewNoopInstance())

		localLib = model.Library{Name: "本地音乐", Path: "localgw:///music"}
		Expect(ds.Library(ctx).Put(&localLib)).To(Succeed())
		cloudLib = model.Library{Name: "网盘音乐", Path: "openlist://127.0.0.1:1/music"}
		Expect(ds.Library(ctx).Put(&cloudLib)).To(Succeed())
	})

	When("the gateway is disabled from the 云源管理面板", func() {
		It("skips the cloud library without touching its data and keeps local scanning unchanged", func() {
			Expect(ds.Property(ctx).Put(consts.CloudSourceGatewayStatesKey, `{"gw":false}`)).To(Succeed())

			_, err := s.ScanAll(ctx, false)
			Expect(err).ToNot(HaveOccurred())

			// Local library scanned as usual (红线: 本地库行为零变化).
			localFiles, err := ds.MediaFile(ctx).GetAll(model.QueryOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(slice.Filter(localFiles, func(mf model.MediaFile) bool { return mf.LibraryID == localLib.ID })).
				To(HaveLen(1))

			// Cloud library untouched: not prepared for scan, nothing imported.
			reloaded, err := ds.Library(ctx).Get(cloudLib.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(reloaded.LastScanStartedAt.IsZero()).To(BeTrue(), "disabled gateway library must not be prepared/scanned")
			Expect(reloaded.LastScanAt.IsZero()).To(BeTrue(), "disabled gateway library must not record a scan")

			cloudFiles := slice.Filter(localFiles, func(mf model.MediaFile) bool { return mf.LibraryID == cloudLib.ID })
			Expect(cloudFiles).To(BeEmpty(), "no media files may be imported from a disabled gateway")
		})
	})

	When("the gateway is enabled", func() {
		It("keeps the library in the scan set (it is only stopped by the 启停 switch)", func() {
			// Explicitly re-enable: state must not depend on what the previous spec left behind.
			Expect(ds.Property(ctx).Put(consts.CloudSourceGatewayStatesKey, `{"gw":true}`)).To(Succeed())
			// Enabled state: the cloud library must at least be *prepared* for scanning
			// (LastScanStartedAt set). The actual listing fails against the closed port
			// and is reported as a scan error — but that is gateway health, not
			// the 启停 switch; here we only lock the switch semantics.
			_, _ = s.ScanAll(ctx, false)

			reloaded, err := ds.Library(ctx).Get(cloudLib.ID)
			Expect(err).ToNot(HaveOccurred())
			// ScanBegin/ScanEnd bookkeeping ran for the library (ScanEnd resets
			// last_scan_started_at and stamps last_scan_at) — only skipped libraries
			// end a scan with LastScanAt still zero.
			Expect(reloaded.LastScanAt.IsZero()).To(BeFalse(), "enabled gateway libraries stay in the scan set")
		})
	})
})
