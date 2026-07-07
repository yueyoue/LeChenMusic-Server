package scanner

import (
	"context"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/scanner/metadata"
	"github.com/navidrome/navidrome/utils/chain"
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

func (p *phaseAudiobooks) producer() <-chan *model.Folder {
	out := make(chan *model.Folder)
	go func() {
		defer close(out)
		// Only scan libraries with media_type = "audiobook"
		for _, lib := range p.state.libraries {
			if lib.MediaType == "audiobook" {
				log.Info(p.ctx, "Scanner: Audiobook library found", "library", lib.Name, "path", lib.Path)
				scanner := NewAudiobookScanner(p.ds)
				if err := scanner.ScanLibrary(p.ctx, lib); err != nil {
					log.Error(p.ctx, "Scanner: Audiobook scan failed", "library", lib.Name, err)
				}
			}
		}
	}()
	return out
}

func (p *phaseAudiobooks) stages() []chain.Stage[*model.Folder] {
	return nil // No additional stages needed
}

func (p *phaseAudiobooks) finalize(err error) error {
	return err
}

// Ensure phaseAudiobooks implements the phase interface
var _ phase[*model.Folder] = (*phaseAudiobooks)(nil)

// [LeChenMusic-END:audiobook]
