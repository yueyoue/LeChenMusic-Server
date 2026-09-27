package persistence

import (
	"context"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pocketbase/dbx"
)

var _ = Describe("AudiobookRepository", func() {
	var repo model.AudiobookRepository
	var libRepo model.LibraryRepository
	var ctx context.Context
	var conn *dbx.DB
	var testLib model.Library

	cleanupTestLib := func() {
		// Delete by path (not id): after a crashed run the leftover row has an unknown id.
		_, err := conn.NewQuery("DELETE FROM audiobook WHERE library_id IN (SELECT id FROM library WHERE path = {:path})").
			Bind(dbx.Params{"path": "/audiobooks-test"}).Execute()
		Expect(err).ToNot(HaveOccurred())
		_, err = conn.NewQuery("DELETE FROM library WHERE path = {:path}").
			Bind(dbx.Params{"path": "/audiobooks-test"}).Execute()
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		ctx = request.WithUser(log.NewContext(context.TODO()), model.User{ID: "userid"})
		conn = GetDBXBuilder()
		repo = NewAudiobookRepository(ctx, conn)
		libRepo = NewLibraryRepository(ctx, conn)

		// Self-heal: wipe leftovers from previous runs first (the suite DB persists),
		// otherwise a stale row trips the UNIQUE(path) constraint on every re-run.
		cleanupTestLib()

		testLib = model.Library{Name: "AB Test Lib", Path: "/audiobooks-test"}
		Expect(libRepo.Put(&testLib)).To(Succeed())
	})

	AfterEach(func() {
		cleanupTestLib()
	})

	newBook := func(title string) *model.Audiobook {
		book := &model.Audiobook{
			Title:     title,
			LibraryID: testLib.ID,
			Path:      "书名/" + title,
			Hash:      "hash-" + title,
		}
		Expect(repo.Put(book)).To(Succeed())
		return book
	}

	// Regression guard: Get used to JOIN library without qualifying `id`, which made the
	// WHERE clause ambiguous ("ambiguous column name: id") and turned every per-book
	// endpoint (detail/play/cover upload) into a 404.
	Describe("Get", func() {
		It("returns the book and enriches LibraryPath from the library table", func() {
			book := newBook("背后有人")

			got, err := repo.Get(book.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(got.ID).To(Equal(book.ID))
			Expect(got.Title).To(Equal("背后有人"))
			Expect(got.LibraryPath).To(Equal("/audiobooks-test"))
		})

		It("returns ErrNotFound for a missing id", func() {
			_, err := repo.Get("does-not-exist")
			Expect(err).To(MatchError(model.ErrNotFound))
		})
	})

	Describe("GetAll", func() {
		It("returns books with LibraryPath filled in", func() {
			newBook("书A")
			newBook("书B")

			all, err := repo.GetAll()
			Expect(err).ToNot(HaveOccurred())
			var titles []string
			for _, b := range all {
				if b.LibraryID == testLib.ID {
					titles = append(titles, b.Title)
					Expect(b.LibraryPath).To(Equal("/audiobooks-test"))
				}
			}
			Expect(titles).To(ContainElements("书A", "书B"))
		})
	})
})
