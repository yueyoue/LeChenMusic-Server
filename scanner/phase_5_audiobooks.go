package scanner

import (
	"context"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	ppl "github.com/google/go-pipeline/pkg/pipeline"
)

// [LeChenMusic-START:audiobook]

func createPhaseAudiobooks(ctx context.Context, state *scanState, ds model.DataStore) *phaseAudiobooks {
	return &phaseAudiobooks{ctx: ctx, state: state, ds: ds}
}

type phaseAudiobooks struct {
	ctx   context.Context
	state *scanState
	ds    model.DataStore
}

func (p *phaseAudiobooks) description() string { return "Scan audiobook libraries" }

func (p *phaseAudiobooks) producer() ppl.Producer[*model.Folder] {
	return ppl.NewProducer(func(put func(entry *model.Folder)) error {
		for _, lib := range p.state.libraries {
			if lib.MediaType == "audiobook" {
				log.Info(p.ctx, "Scanner: Audiobook library found", "library", lib.Name, "path", lib.Path)
				scanner := NewAudiobookScanner(p.ds)
				if err := scanner.ScanLibrary(p.ctx, lib); err != nil {
					log.Error(p.ctx, "Scanner: Audiobook scan failed", "library", lib.Name, err)
				}
				// The audiobook scanner is the only writer of these rows (it creates, updates
				// and now also purges vanished books), so always ask for a library stats
				// refresh: a quick scan would otherwise leave 专辑数/歌曲数 stale after a
				// shelf moved away. The refresh is a handful of COUNT queries.
				p.state.changesDetected.Store(true)
			}
		}
		return nil
	}, ppl.Name("scan audiobook libraries"))
}

func (p *phaseAudiobooks) stages() []ppl.Stage[*model.Folder] {
	return nil
}

func (p *phaseAudiobooks) finalize(err error) error {
	return err
}

// [LeChenMusic-END:audiobook]
