package subsonic

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/subsonic/responses"
)

// [LeChenMusic-START:audiobook-subsonic]

func (api *Router) GetAudiobooks(r *http.Request) (*responses.Subsonic, error) {
	repo := api.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		return nil, newError(responses.ErrorGeneric, err.Error())
	}

	response := newResponse()
	var audiobooks []responses.AudiobookID3
	for _, b := range books {
		chapters, _ := repo.GetChapters(b.ID)
		audiobooks = append(audiobooks, toAudiobookID3(b, chapters))
	}
	response.Audiobooks = &responses.Audiobooks{Audiobook: audiobooks}
	return response, nil
}

func (api *Router) GetAudiobook(r *http.Request) (*responses.Subsonic, error) {
	p := r.FormValue("id")
	if p == "" {
		return nil, newError(responses.ErrorMissingParameter, "id is required")
	}
	repo := api.ds.Audiobook(r.Context())
	book, err := repo.Get(p)
	if err != nil {
		return nil, newError(responses.ErrorDataNotFound, "audiobook not found")
	}
	chapters, _ := repo.GetChapters(p)
	response := newResponse()
	ab := toAudiobookID3(*book, chapters)
	response.Audiobook = &ab
	return response, nil
}

func (api *Router) GetAudiobookChapters(r *http.Request) (*responses.Subsonic, error) {
	p := r.FormValue("id")
	if p == "" {
		return nil, newError(responses.ErrorMissingParameter, "id is required")
	}
	repo := api.ds.Audiobook(r.Context())
	chapters, err := repo.GetChapters(p)
	if err != nil {
		return nil, newError(responses.ErrorDataNotFound, "audiobook not found")
	}
	response := newResponse()
	var children []responses.Child
	for _, ch := range chapters {
		children = append(children, toAudiobookChapterChild(ch))
	}
	response.AudiobookChapters = &responses.AudiobookChapters{Chapter: children}
	return response, nil
}

func (api *Router) StreamAudiobookChapter(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	bookID := r.FormValue("id")
	chapterID := r.FormValue("chapterId")
	if bookID == "" || chapterID == "" {
		return nil, newError(responses.ErrorMissingParameter, "id and chapterId are required")
	}

	repo := api.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		return nil, newError(responses.ErrorDataNotFound, "audiobook not found")
	}
	chapter, err := repo.GetChapter(chapterID)
	if err != nil {
		return nil, newError(responses.ErrorDataNotFound, "chapter not found")
	}
	lib, err := api.ds.Library(r.Context()).Get(book.LibraryID)
	if err != nil {
		return nil, newError(responses.ErrorDataNotFound, "library not found")
	}
	filePath := filepath.Join(lib.Path, book.Path, chapter.Path)
	http.ServeFile(w, r, filePath)
	return nil, nil
}

func toAudiobookID3(book model.Audiobook, chapters model.AudiobookChapters) responses.AudiobookID3 {
	var childChapters []responses.Child
	for _, ch := range chapters {
		childChapters = append(childChapters, toAudiobookChapterChild(ch))
	}
	return responses.AudiobookID3{
		ID:            book.ID,
		Title:         book.Title,
		Author:        book.Author,
		Narrator:      book.Narrator,
		CoverArt:      book.ID,
		Duration:      book.TotalDuration,
		Genre:         book.Genre,
		Year:          book.Year,
		Series:        book.Series,
		ChapterCount:  book.ChapterCount,
		Chapter:       childChapters,
		CreatedAt:     book.CreatedAt.Format("2006-01-02T15:04:05"),
	}
}

func toAudiobookChapterChild(ch model.AudiobookChapter) responses.Child {
	return responses.Child{
		Id:          ch.ID,
		Title:       ch.Title,
		Track:       int32(ch.ChapterNumber),
		Duration:    int32(ch.Duration),
		ContentType: getContentType(ch.Format),
		Suffix:      ch.Format,
		Type:        "audiobook",
	}
}

func getContentType(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "m4a", "m4b":
		return "audio/mp4"
	case "flac":
		return "audio/flac"
	case "ogg", "opus":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "aac":
		return "audio/aac"
	case "wma":
		return "audio/x-ms-wma"
	default:
		return "audio/mpeg"
	}
}

// [LeChenMusic-END:audiobook-subsonic]
