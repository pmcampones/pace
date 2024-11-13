package agreementCommonSubset

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/samber/mo"
	aba "pace/asynchronousBinaryAgreement"
	"pace/utils"
)

type BKR2 struct {
	id          uuid.UUID
	f           uint
	acceptors   []*proposalAcceptor
	resultsChan chan lo.Tuple2[mo.Option[[]byte], uint]
	results     [][]byte
	output      chan [][]byte
}

func NewBKR2(id uuid.UUID, f uint, proposers []uuid.UUID, abaChan *aba.AbaChannel) *BKR2 {
	b2 := &BKR2{
		id:          id,
		f:           f,
		acceptors:   computeAcceptors(id, proposers, abaChan),
		resultsChan: make(chan lo.Tuple2[mo.Option[[]byte], uint], len(proposers)),
		results:     make([][]byte, len(proposers)),
		output:      make(chan [][]byte, 1),
	}
	go b2.processResponses()
	for i, acceptor := range b2.acceptors {
		go b2.waitAcceptorResponse(acceptor, uint(i))
	}
	return b2
}

func computeAcceptors(bkrId uuid.UUID, proposers []uuid.UUID, abaChan *aba.AbaChannel) []*proposalAcceptor {
	return lo.Map(proposers, func(proposer uuid.UUID, _ int) *proposalAcceptor {
		abaId := utils.BytesToUUID(append(bkrId[:], proposer[:]...))
		return newProposalAcceptor(abaId, proposer, abaChan)
	})
}

func (b *BKR2) waitAcceptorResponse(acceptor *proposalAcceptor, idx uint) {
	response := <-acceptor.output
	b.resultsChan <- lo.Tuple2[mo.Option[[]byte], uint]{A: response, B: idx}
}

func (b *BKR2) processResponses() {
	for i := uint(0); i < uint(len(b.acceptors)); i++ {
		response := <-b.resultsChan
		proposal, idx := response.Unpack()
		b.results[idx] = proposal.OrEmpty()
		if proposal.IsPresent() && len(b.getAccepted()) == len(b.acceptors)-int(b.f) {
			for _, a := range b.acceptors {
				if !a.hasProposed() {
					_ = a.rejectProposal()
				}
			}
		}
	}
	b.output <- b.getAccepted()
}

func (b *BKR2) getAccepted() [][]byte {
	return lo.Filter(b.results, func(r []byte, _ int) bool { return r != nil })
}

func (b *BKR2) receiveInput(input []byte, proposer uuid.UUID) error {
	for _, a := range b.acceptors {
		if a.proposer == proposer {
			return a.submitInput(input)
		}
	}
	return fmt.Errorf("unable to find acceptor for proposer %s", proposer)
}
