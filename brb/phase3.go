package brb

import (
	"fmt"
	"github.com/google/uuid"
)

type brbPhase3Handler struct {
	data       *brbData
	outputChan chan<- []byte
}

func newPhase3Handler(data *brbData, output chan<- []byte) *brbPhase3Handler {
	return &brbPhase3Handler{
		data:       data,
		outputChan: output,
	}
}

func (b *brbPhase3Handler) handleReady(msg []byte, id uuid.UUID) error {
	instanceLogger.Debug("processing ready message on phase 3")
	numReadies, ok := b.data.readies[id]
	if !ok {
		return fmt.Errorf("unable to find id for ready message in phase 3")
	}
	if numReadies == b.data.n-b.data.f {
		instanceLogger.Info("sending output message")
		b.outputChan <- msg
	}
	return nil
}
